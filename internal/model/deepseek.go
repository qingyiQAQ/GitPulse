// deepseek.go 提供 model.Client 的 DeepSeek 实现。
//
// 阶段一严格遵循「仅标准库」约束，用 net/http 手写 DeepSeek 的 OpenAI 兼容
// Chat Completions 客户端，而非引入第三方 SDK：这是本项目刻意为之的学习目标——
// 只有亲手实现一次 HTTP 客户端，才能吃透协议细节（请求头、错误码、重试语义、
// 结构化输出约束），也为后续「换厂商 / 加降级」打下基础。
//
// 参考：https://api-docs.deepseek.com/（Chat Completions API，OpenAI 兼容格式）
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"
)

// 常量集中管理，避免魔法值散落在代码里。
const (
	// defaultDeepSeekBaseURL 是 DeepSeek 官方 API 基址。
	// 也可用 https://api.deepseek.com/v1（v1 与模型版本无关），本项目用不带 /v1 的简式。
	defaultDeepSeekBaseURL = "https://api.deepseek.com"

	// defaultDeepSeekModel 是默认模型：DeepSeek V4 Pro（当前旗舰，通用能力最强）。
	// 备用选择 deepseek-flash（更轻更快）可支撑分层路由：flash 做相关性分类、v4-pro 做摘要。
	// 注意：旧模型名 deepseek-chat / deepseek-reasoner 已于 2026-07 弃用，勿再使用。
	defaultDeepSeekModel = "deepseek-v4-pro"

	// chatCompletionsPath 是 Chat Completions API 的路径（OpenAI 兼容）。
	chatCompletionsPath = "/chat/completions"

	// defaultMaxTokens 是单次补全的最大输出 token 数。
	// 本项目输出都很短（相关性判断 ~百 token、摘要 ~数百 token），2048 已留足余量。
	// 注意：该字段只限制上限、按实际用量计费，故无需抠紧。
	defaultMaxTokens = 2048

	// defaultHTTPTimeout 是单次 HTTP 请求的总体超时。
	// 阶段一输出很短，60s 绰绰有余；若未来引入流式/长输出，应改为「无总超时 + 读超时」。
	defaultHTTPTimeout = 60 * time.Second

	// defaultMaxRetries 是「瞬态错误」（429/5xx/网络）的最大重试次数，即共 1+N 次尝试。
	defaultMaxRetries = 3
)

// DeepSeekConfig 是 DeepSeekClient 的构造参数。
//
// 除 APIKey 业务上必填外，其余字段均可选（有默认值）。APIKey 的必填校验
// 统一放到 config.Load 中做，构造器不因缺 key 报错——方便测试注入空 key 的 Mock。
type DeepSeekConfig struct {
	// APIKey 是 DeepSeek 的访问密钥（Authorization: Bearer <key>）。
	// 必须从环境变量注入，绝不硬编码、绝不入库。
	APIKey string

	// Model 指定模型 ID，默认 deepseek-v4-pro。
	Model string

	// BaseURL 指定 API 基址，默认官方地址；测试时可指向 httptest.Server 做 Mock。
	BaseURL string

	// MaxTokens 指定单次补全最大输出 token 数，默认 2048。
	MaxTokens int

	// HTTPClient 允许注入自定义 http.Client（代理、自定义超时、Mock 传输层等）。
	// 这是「依赖注入」原则的延续：测试可借此替换传输层，不发真实网络请求。
	HTTPClient *http.Client

	// MaxRetries 指定瞬态错误的最大重试次数，默认 3。
	MaxRetries int
}

// DeepSeekClient 是基于 DeepSeek Chat Completions API 的 model.Client 实现。
//
// 它把「裸 HTTP 细节」封闭在本结构体内，对外只暴露 model.Client 接口，
// 上层（agent、tools）因此完全感知不到底层是哪个厂商、走的什么协议。
type DeepSeekClient struct {
	apiKey     string
	model      string
	baseURL    string
	maxTokens  int
	httpClient *http.Client
	maxRetries int
	// backoff 是退避时长函数，默认 backoffDelay；测试可注入 0 退避以加速。
	backoff func(attempt int) time.Duration
}

// NewDeepSeekClient 构造 DeepSeekClient，未显式提供的参数一律落到默认值。
//
// 入参：
//   - cfg：构造参数（各字段含义见 DeepSeekConfig）。其中 APIKey 业务上必填；
//     BaseURL / Model / MaxTokens / HTTPClient / MaxRetries 留空时分别落到对应默认值。
//
// 返回值：
//   - *DeepSeekClient：已按默认值补全、可直接调用 Complete / CompleteJSON 的客户端。
func NewDeepSeekClient(cfg DeepSeekConfig) *DeepSeekClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultDeepSeekBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultDeepSeekModel
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = defaultMaxTokens
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	return &DeepSeekClient{
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		maxTokens:  cfg.MaxTokens,
		httpClient: cfg.HTTPClient,
		maxRetries: cfg.MaxRetries,
		backoff:    backoffDelay,
	}
}

// Complete 实现非结构化文本补全：把 prompt 交给模型，取回其自由生成的文本。
//
// 阶段一用它兜底：当结构化输出/工具链路整体失败时，降级为「一句人话」描述给用户。
//
// 入参：
//   - ctx：请求上下文，用于超时控制与取消传播（模型调用耗时较长，务必层层传递）。
//   - prompt：完整的提示词文本，直接作为 user 消息发送给模型。
//
// 返回值：
//   - string：模型生成的文本内容（非空）。
//   - error：请求失败、响应异常或上下文被取消时返回；nil 表示成功。
func (c *DeepSeekClient) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := c.doRequest(ctx, prompt, nil)
	if err != nil {
		return "", err
	}
	return extractText(resp)
}

// CompleteJSON 实现结构化补全：强制模型输出符合约定 JSON Schema 的 JSON，
// 并反序列化到 result（必须为指向结构体的非 nil 指针）。
//
// 失败语义（接口契约的核心）：
//   - 请求失败（网络/限流/4xx/5xx）→ 返回 error；
//   - 模型输出非法 JSON / 与目标结构不符 → 返回 error。
//
// 上层据此执行降级：重试 → 简化 → 人工复核。
//
// 入参：
//   - ctx：请求上下文，用于超时控制与取消传播。
//   - prompt：任务描述提示词。本方法会自动把由 result 类型推导出的 JSON Schema
//     附加到提示词末尾，约束模型按结构输出，调用方无需手工拼 schema。
//   - result：指向目标结构体的指针（如 *schemas.RelevanceVerdict）。
//     成功时模型输出被反序列化写入其中，即「原地填充」；失败时其值保持不变。
//
// 返回值：
//   - error：nil 表示成功且 result 已被填充；非 nil 表示请求失败、result 类型
//     非法（非指针/nil）、或模型输出非法 JSON / 结构不符，由上层执行降级。
func (c *DeepSeekClient) CompleteJSON(ctx context.Context, prompt string, result any) error {
	if result == nil {
		return errors.New("model: CompleteJSON 的 result 不能为 nil")
	}
	rv := reflect.ValueOf(result)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errors.New("model: CompleteJSON 的 result 必须是指向结构体的非 nil 指针")
	}

	// 从目标类型推导 JSON Schema（反射），保证 schema 与 struct 单一数据源、不漂移。
	schema, err := jsonSchemaFromType(rv.Elem().Type())
	if err != nil {
		return fmt.Errorf("model: 从 %T 推导 JSON Schema 失败: %w", result, err)
	}

	resp, err := c.doRequest(ctx, prompt, schema)
	if err != nil {
		return err
	}

	text, err := extractText(resp)
	if err != nil {
		return err
	}

	// 严格解码：拒绝未知字段、拒绝 JSON 后的多余内容。
	// 这是「结构不符必须报错」契约的具体落实——宁可误报触发降级，也不静默容忍脏数据。
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(result); err != nil {
		return fmt.Errorf("model: 模型输出非法 JSON 或与目标结构不符: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("model: 模型输出在 JSON 之后含有多余内容")
	}
	return nil
}

// —— 以下是 Chat Completions API 的请求/响应载荷定义 ——
//
// 只声明本项目用到的字段：Go 的 json 反序列化会忽略未声明字段，
// 因此无需穷举 API 全部字段，保持最小可用面。

// messageParam 表示一条消息。
// content 为纯文本；role 支持 system / user / assistant（阶段一只用前两者）。
type messageParam struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responseFormat 请求 JSON 输出。
// 注意：DeepSeek 只支持 {"type":"json_object"}（强制合法 JSON），
// 不支持 OpenAI 的严格 json_schema 类型——因此结构约束只能「写进提示词」软引导。
type responseFormat struct {
	Type string `json:"type"` // "json_object"
}

// chatRequest 是一次 Chat Completions 请求体。
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []messageParam  `json:"messages"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// choice 是响应里的一个候选。
type choice struct {
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"` // stop / length / content_filter / ...
}

// message 是候选里的回复内容。
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse 是一次 Chat Completions 成功响应体。
type chatResponse struct {
	Choices []choice `json:"choices"`
	// Usage 预留：观测输入/输出 token 消耗（可观测性），阶段一暂不对外暴露。
	Usage usage `json:"usage"`
}

// usage 是 token 消耗统计（OpenAI 字段命名）。
type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// apiErrorEnvelope 是 DeepSeek 的非 2xx 错误响应体（OpenAI 格式）。
// 形如 {"error":{"message":"...","type":"..."}}。
type apiErrorEnvelope struct {
	Error apiErrorBody `json:"error"`
}

// apiErrorBody 是错误信封里的 error 子对象。
type apiErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"` // authentication_error / rate_limit_error / ...
}

// rawResponse 是一次原始 HTTP 往返的结果（status + 已读入内存的 body）。
// 抽象出它，是为了在 doRequest 里干净地分离「成功解析」与「错误处理」两条路径。
type rawResponse struct {
	StatusCode int
	Body       []byte
}

// doRequest 发起一次 Chat Completions 请求并返回解析后的响应。
//
// 当 schema 为 nil 时是非结构化补全（单条 user 消息）；
// 非 nil 时附加 response_format=json_object，并把 JSON Schema 写进提示词
// （system + user 两条消息）软约束结构。
// 内置「瞬态错误重试」：网络错误、429（限流）、5xx（500 服务器故障 / 503 繁忙）
// 按指数退避重试，其余（400/401/402/422 业务错误）立即返回，不做无意义重试。
//
// 入参：
//   - ctx：请求上下文；重试退避期间仍会响应其取消，让上层能及时叫停。
//   - prompt：用户提示词正文。
//   - schema：目标 JSON Schema；nil 表示非结构化补全，非 nil 表示结构化补全。
//
// 返回值：
//   - *chatResponse：解析后的成功响应（含 choices 与 usage），供上层提取正文。
//   - error：重试耗尽仍失败、或遇到不可重试的业务错误时返回。
func (c *DeepSeekClient) doRequest(ctx context.Context, prompt string, schema map[string]any) (*chatResponse, error) {
	messages := []messageParam{{Role: "user", Content: prompt}}
	var respFormat *responseFormat

	if schema != nil {
		// DeepSeek 无原生 JSON Schema 约束，只能靠 response_format=json_object 保证「合法 JSON」，
		// 再通过提示词软约束「字段结构」。这里把 schema 序列化后注入 user 消息，
		// 并用 system 消息强调「只输出 JSON」，满足 json_object 模式要求提示词含 "json" 字样的前提。
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("model: 序列化 JSON Schema 失败: %w", err)
		}
		messages = []messageParam{
			{
				Role:    "system",
				Content: "你是一个 JSON 输出引擎：只输出一个合法的 JSON 对象，不输出任何解释、前后缀或 Markdown 代码块。",
			},
			{
				Role:    "user",
				Content: prompt + "\n\n请严格按照以下 JSON Schema 输出（字段名与类型必须一致）：\n" + string(schemaJSON),
			},
		}
		respFormat = &responseFormat{Type: "json_object"}
	}

	req := chatRequest{
		Model:          c.model,
		Messages:       messages,
		MaxTokens:      c.maxTokens,
		ResponseFormat: respFormat,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("model: 序列化请求体失败: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// 非首次尝试先退避等待；退避期间响应 context 取消，让上层能及时叫停。
		if attempt > 0 {
			if err := sleepWithContext(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}

		resp, err := c.roundTrip(ctx, body)
		if err != nil {
			lastErr = err
			// 网络层错误一律视为瞬态，继续重试。
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var msg chatResponse
			if err := json.Unmarshal(resp.Body, &msg); err != nil {
				return nil, fmt.Errorf("model: 解析成功响应失败: %w", err)
			}
			return &msg, nil
		}

		apiErr := parseAPIError(resp.StatusCode, resp.Body)
		lastErr = apiErr
		// 只有瞬态错误才重试；业务错误（鉴权失败、余额不足、参数非法等）立即返回。
		if !apiErr.IsRetryable() {
			return nil, apiErr
		}
	}
	return nil, fmt.Errorf("model: 重试 %d 次后仍失败: %w", c.maxRetries, lastErr)
}

// roundTrip 执行一次原始 HTTP POST，读取并关闭响应体。
// 返回原始 status 与 body，不做任何业务层解释。
//
// 入参：
//   - ctx：请求上下文（经 http.NewRequestWithContext 绑定到本次请求）。
//   - body：已序列化好的 JSON 请求体字节。
//
// 返回值：
//   - *rawResponse：HTTP 状态码 + 响应体原始字节。
//   - error：构造请求、发送或读取响应体失败时返回（均属网络层错误，视为瞬态）。
func (c *DeepSeekClient) roundTrip(ctx context.Context, body []byte) (*rawResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+chatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("model: 构造请求失败: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("model: 发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("model: 读取响应体失败: %w", err)
	}
	return &rawResponse{StatusCode: resp.StatusCode, Body: respBody}, nil
}

// extractText 从响应中提取纯文本正文。
//
// 必须先看 finish_reason（响应可能 200 但内容异常）：
//   - content_filter：内容被过滤，返回 error；
//   - length：输出被截断，返回 error（提示增大 max_tokens）。
//
// 入参：
//   - resp：已解析的成功响应。
//
// 返回值：
//   - string：choices[0].message.content 去除首尾空白后的正文。
//   - error：无 choices、被内容过滤、被截断或正文为空时返回。
func extractText(resp *chatResponse) (string, error) {
	if len(resp.Choices) == 0 {
		return "", errors.New("model: 响应中无 choices")
	}
	choice := resp.Choices[0]
	switch choice.FinishReason {
	case "content_filter":
		return "", errors.New("model: 输出被内容过滤（finish_reason=content_filter）")
	case "length":
		return "", errors.New("model: 输出被 max_tokens 截断，需增大 max_tokens")
	}

	text := strings.TrimSpace(choice.Message.Content)
	if text == "" {
		return "", errors.New("model: 响应中未包含文本内容")
	}
	return text, nil
}

// sleepWithContext 在退避等待期间保持对 context 的响应，取消时立即返回。
//
// 入参：
//   - ctx：取消信号来源。
//   - d：需要等待的时长。
//
// 返回值：
//   - error：ctx 在等待期间被取消时返回 ctx.Err()；正常等到超时返回 nil。
func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// backoffDelay 计算第 attempt 次重试前的退避等待时长（指数退避）。
// 第 1 次重试 1s，之后每轮翻倍，封顶 30s。
// 阶段一采用确定性退避（无抖动）；生产环境应加随机抖动以避免「惊群」，
// 那是阶段三「工程化/治理」再补的细节。
//
// 入参：
//   - attempt：本次重试的序号（从 1 开始，1 表示第一次重试）。
//
// 返回值：
//   - time.Duration：本次重试前应等待的时长（1s 起、逐轮翻倍、封顶 30s）。
func backoffDelay(attempt int) time.Duration {
	d := time.Second << uint(attempt-1) // 1s, 2s, 4s, ...
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// APIError 表示 DeepSeek 返回的非 2xx 业务/服务错误。
//
// 将其暴露为具体类型（而非 fmt.Errorf 的字符串），是为了让上层
// 能通过 errors.As 判定错误类别，从而执行差异化降级（如 429 可换模型重试）。
type APIError struct {
	// StatusCode 是 HTTP 状态码。
	StatusCode int
	// Type 是 DeepSeek 错误类型（如 authentication_error / rate_limit_error）。
	Type string
	// Message 是 DeepSeek 返回的原文错误信息。
	Message string
}

// Error 实现 error 接口，返回人类可读的错误描述。
//
// 入参：无。
//
// 返回值：
//   - string：形如 "deepseek API 错误 (HTTP <状态码>, <类型>): <信息>" 的文本；
//     Message 为空时省略冒号后的信息段。
func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("deepseek API 错误 (HTTP %d, %s)", e.StatusCode, e.Type)
	}
	return fmt.Sprintf("deepseek API 错误 (HTTP %d, %s): %s", e.StatusCode, e.Type, e.Message)
}

// IsRetryable 报告该错误是否属于「瞬态、可重试」。
//
// 语义与 DeepSeek 文档一致：
//   - 429（限流）、408（超时）、500/503（服务器故障/繁忙）→ 可重试（指数退避）；
//   - 400（格式错误）、401（鉴权失败）、402（余额不足）、422（参数非法）→ 不可重试。
//
// 入参：无。
//
// 返回值：
//   - bool：true 表示瞬态、重试有望成功；false 表示重试只会得到同样的错。
func (e *APIError) IsRetryable() bool {
	switch {
	case e.StatusCode == http.StatusTooManyRequests: // 429
		return true
	case e.StatusCode == http.StatusRequestTimeout: // 408
		return true
	case e.StatusCode >= 500 && e.StatusCode <= 599: // 500 / 503
		return true
	default:
		return false
	}
}

// parseAPIError 解析非 2xx 响应体，构造 APIError。
// 若响应体不符合标准错误信封（如网关返回的 HTML），退化为携带原始文本的错误，
// 保证错误信息不丢失、便于排障。
//
// 入参：
//   - status：HTTP 状态码。
//   - body：响应体原始字节。
//
// 返回值：
//   - *APIError：由状态码 + 解析出的 type/message（解析失败则用原始文本）构造的错误。
func parseAPIError(status int, body []byte) *APIError {
	var env apiErrorEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		return &APIError{StatusCode: status, Type: env.Error.Type, Message: env.Error.Message}
	}
	return &APIError{
		StatusCode: status,
		Type:       "unknown_error",
		Message:    strings.TrimSpace(string(body)),
	}
}

// jsonSchemaFromType 通过反射把 Go struct 类型推导为 JSON Schema。
//
// 设计动机：结构化输出的 schema 若与 struct 分两处手写，必然漂移。
// 以 struct（schemas 包，单一数据源）为准自动生成 schema，从根上消除漂移。
//
// 阶段一只覆盖项目实际用到的类型子集（见 schemaNode）。不支持的类型直接报错，
// 而非静默产出错误 schema。enum（Category 的取值约束）留待后续阶段：
// 需要给具名类型挂枚举常量元数据。
//
// 入参：
//   - t：目标结构体类型（可传指针，内部会自动解引用）。
//
// 返回值：
//   - map[string]any：根类型为 object 的 JSON Schema。
//   - error：t 不是 struct 类型，或某个字段类型不受支持时返回。
func jsonSchemaFromType(t reflect.Type) (map[string]any, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("根类型必须是指向 struct 的指针，实际为 %s", t.Kind())
	}
	return objectSchema(t)
}

// objectSchema 为 struct 生成 object 类型的 JSON Schema。
//
// 入参：
//   - t：结构体类型（须已解引用指针）。
//
// 返回值：
//   - map[string]any：含 type/properties/required/additionalProperties 的 object schema。
//   - error：某个字段类型不受支持时返回（错误信息会标注具体字段名）。
func objectSchema(t reflect.Type) (map[string]any, error) {
	props := map[string]any{}
	required := make([]string, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		// 跳过未导出字段（反射无法安全序列化）。
		if f.PkgPath != "" {
			continue
		}

		// 字段名以 json tag 为准；tag 为 "-" 表示不参与序列化。
		name := f.Name
		if tag := f.Tag.Get("json"); tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			name = parts[0]
		}

		child, err := schemaNode(f.Type)
		if err != nil {
			return nil, fmt.Errorf("字段 %s: %w", name, err)
		}
		props[name] = child
		required = append(required, name)
	}

	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}, nil
}

// schemaNode 为任意（已解引用指针的）类型生成 JSON Schema 节点。
//
// 入参：
//   - t：字段类型（可为指针，内部自动解引用）。
//
// 返回值：
//   - map[string]any：该类型对应的 schema 节点（string/boolean/integer/number/array/object）。
//   - error：类型不受支持（chan/func/map/interface 等）时返回。
func schemaNode(t reflect.Type) (map[string]any, error) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	case reflect.Slice, reflect.Array:
		item, err := schemaNode(t.Elem())
		if err != nil {
			return nil, fmt.Errorf("数组元素: %w", err)
		}
		return map[string]any{"type": "array", "items": item}, nil
	case reflect.Struct:
		return objectSchema(t)
	default:
		return nil, fmt.Errorf("不支持的字段类型 %s", t.Kind())
	}
}
