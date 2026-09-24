package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gitpulse/gitpulse/internal/schemas"
)

// GitHub 相关的常量集中管理，避免魔法值散落。
const (
	// githubAPIBase 是 GitHub REST API 的基址。
	githubAPIBase = "https://api.github.com"

	// githubAPIVersion 通过 X-GitHub-Api-Version 头锁定 API 版本，避免接口漂移。
	githubAPIVersion = "2022-11-28"

	// githubAccept 是 GitHub 推荐的 Accept 头（JSON 格式 + 特性集）。
	githubAccept = "application/vnd.github+json"

	// trendingWindowDays 是「热门」的时间窗：只看最近 N 天新建的项目，近似 GitHub Trending。
	// 官方 Trending 页无公开 API，这里用「近期新建 + 高 star」作为可复现的近似。
	trendingWindowDays = 7

	// defaultTrendingLimit 是调用方未指定 limit 时的默认抓取数量。
	defaultTrendingLimit = 20

	// defaultHTTPTimeout 是单次 HTTP 请求的总体超时。
	defaultHTTPTimeout = 30 * time.Second

	// defaultMaxRetries 是瞬态错误（限流/5xx/网络）的最大重试次数，即共 1+N 次尝试。
	defaultMaxRetries = 3
)

// GitHubTool 封装 GitHub 数据采集能力（Pipeline 第一步「采集」）。
//
// 子能力：
//   - fetchTrending：抓取热门项目列表（Search API，按创建时间 + star 排序）
//   - fetchDetail：  抓取单个项目的详情（Repo API）
//
// 工程重点：
//   - GitHub Trending 无官方 API，阶段一用 Search API（created 过滤 + stars 排序）近似；
//   - 限流（Retry-After 退避）与幂等是这里的核心可靠性问题。
type GitHubTool struct {
	// token 是 GitHub 访问令牌（可选，提供后限流额度更高）。
	token string
	// httpClient 允许注入自定义 http.Client（代理、自定义超时、Mock 传输层）。
	// 这是「依赖注入」的延续：测试可借此替换传输层，不发真实网络请求。
	httpClient *http.Client
	// baseURL 默认官方 API 基址；测试可指向 httptest.Server。
	baseURL string
	// maxRetries 是瞬态错误的最大重试次数。
	maxRetries int
	// backoff 是退避时长函数，默认 backoffDelay；测试可注入 0 退避以加速。
	backoff func(attempt int) time.Duration
}

// NewGitHubTool 构造 GitHub 采集工具。
//
// 入参：
//   - token：GitHub 访问令牌（可为空，空值走匿名访问、限流额度更低）。
//
// 返回值：
//   - *GitHubTool：已按默认值（官方基址、30s 超时、3 次重试）补全、可直接调用的工具。
func NewGitHubTool(token string) *GitHubTool {
	return &GitHubTool{
		token:      token,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		baseURL:    githubAPIBase,
		maxRetries: defaultMaxRetries,
		backoff:    backoffDelay,
	}
}

// Name 实现 Tool 接口。
//
// 入参：无。
//
// 返回值：
//   - string：工具唯一标识 "github"，用于注册表索引与 trace 标记。
func (g *GitHubTool) Name() string { return "github" }

// Description 实现 Tool 接口。
//
// 入参：无。
//
// 返回值：
//   - string：喂给模型的功能描述（说明支持 trending / detail 两个动作及其参数）。
func (g *GitHubTool) Description() string {
	return "抓取 GitHub 开源项目数据。支持两个动作：" +
		"trending（抓取热门项目列表，参数 language 可选、limit 可选默认 20）；" +
		"detail（抓取单个仓库详情，参数 owner 与 name 必填）。"
}

// githubArgs 是 Run 方法的 JSON 参数契约，与 LLM function calling 的 arguments 对齐。
type githubArgs struct {
	// Action 指定子动作："trending"（热门列表）| "detail"（单个仓库详情）。
	Action string `json:"action"`
	// Language 是 trending 动作的可选语言过滤（如 "Go"），空表示不限。
	Language string `json:"language"`
	// Limit 是 trending 动作的抓取数量，<=0 使用默认值。
	Limit int `json:"limit"`
	// Owner 是 detail 动作的仓库所有者。
	Owner string `json:"owner"`
	// Name 是 detail 动作的仓库名。
	Name string `json:"name"`
}

// Run 实现 Tool 接口：按 args 中的 action 分发到具体子能力。
//
// 入参：
//   - ctx：请求上下文，用于超时控制与取消传播。
//   - args：JSON 编码的参数（结构见 githubArgs）；为空时按默认参数处理。
//
// 返回值：
//   - json.RawMessage：动作结果的 JSON 编码（trending 为仓库数组、detail 为单个仓库）。
//   - error：参数非法、动作未知或请求失败时返回；nil 表示成功。
func (g *GitHubTool) Run(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a githubArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, fmt.Errorf("tools: 解析 github 工具参数失败: %w", err)
		}
	}

	switch a.Action {
	case "trending":
		repos, err := g.fetchTrending(ctx, a.Language, a.Limit)
		if err != nil {
			return nil, err
		}
		return json.Marshal(repos)

	case "detail":
		if a.Owner == "" || a.Name == "" {
			return nil, errors.New("tools: detail 动作需要 owner 与 name 两个参数")
		}
		repo, err := g.fetchDetail(ctx, a.Owner, a.Name)
		if err != nil {
			return nil, err
		}
		return json.Marshal(repo)

	default:
		return nil, fmt.Errorf("tools: 未知的 github 动作 %q（支持 trending / detail）", a.Action)
	}
}

// fetchTrending 抓取热门项目列表。
//
// 用 Search API 的「最近 N 天新建 + 按 star 降序」近似 GitHub Trending，
// 保证可复现、可用官方 API（而非抓取易碎的 HTML 页面）。
//
// 入参：
//   - ctx：请求上下文。
//   - language：可选语言过滤（如 "Go"）；空字符串表示不限语言。
//   - limit：期望抓取的数量；<=0 使用默认值 defaultTrendingLimit。
//
// 返回值：
//   - []schemas.Repo：按 star 降序的热门项目列表（可能为空，但不为 nil）。
//   - error：请求失败、限流耗尽或解析失败时返回；nil 表示成功。
func (g *GitHubTool) fetchTrending(ctx context.Context, language string, limit int) ([]schemas.Repo, error) {
	if limit <= 0 {
		limit = defaultTrendingLimit
	}

	// 构建搜索查询：只看最近 N 天新建的项目，可选按语言过滤。
	since := time.Now().AddDate(0, 0, -trendingWindowDays).Format("2006-01-02")
	q := "created:>" + since
	if language != "" {
		q += " language:" + language
	}

	// 用 url.Values 拼 query，避免手拼导致转义错误。
	params := url.Values{}
	params.Set("q", q)
	params.Set("sort", "stars")
	params.Set("order", "desc")
	params.Set("per_page", strconv.Itoa(limit))
	endpoint := g.baseURL + "/search/repositories?" + params.Encode()

	var result searchRepositoriesResponse
	if err := g.getJSON(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("tools: 抓取热门项目失败: %w", err)
	}

	repos := make([]schemas.Repo, 0, len(result.Items))
	for _, item := range result.Items {
		repos = append(repos, mapRepo(item))
	}
	return repos, nil
}

// fetchDetail 抓取单个项目的详情（完整描述、topics、语言、star 数等）。
//
// 入参：
//   - ctx：请求上下文。
//   - owner：仓库所有者（如 "avelino"）。
//   - name：仓库名（如 "awesome-go"）。
//
// 返回值：
//   - *schemas.Repo：该仓库的结构化元数据。
//   - error：请求失败或仓库不存在时返回；nil 表示成功。
func (g *GitHubTool) fetchDetail(ctx context.Context, owner, name string) (*schemas.Repo, error) {
	// PathEscape 防止 owner/name 中的特殊字符造成路径注入。
	endpoint := fmt.Sprintf("%s/repos/%s/%s", g.baseURL, url.PathEscape(owner), url.PathEscape(name))

	var r repository
	if err := g.getJSON(ctx, endpoint, &r); err != nil {
		return nil, fmt.Errorf("tools: 抓取仓库 %s/%s 详情失败: %w", owner, name, err)
	}
	repo := mapRepo(r)
	return &repo, nil
}

// getJSON 发起一次 GET 请求并把 JSON 响应反序列化到 out。
//
// 内置「瞬态错误重试」：网络错误、429（限流）、5xx 按退避重试；
// 携带 Retry-After 头时优先按该时长等待（更精准、避免被限流雪崩）；
// 其余 4xx（404 不存在、401 鉴权失败等）立即返回，不做无意义重试。
//
// 入参：
//   - ctx：请求上下文；重试退避期间仍会响应其取消。
//   - endpoint：完整的请求 URL。
//   - out：指向结果结构体的指针，成功时反序列化写入。
//
// 返回值：
//   - error：重试耗尽仍失败、解析失败或遇到不可重试的错误时返回；nil 表示成功。
func (g *GitHubTool) getJSON(ctx context.Context, endpoint string, out any) error {
	var lastErr error
	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, g.waitBeforeRetry(lastErr, attempt)); err != nil {
				return err
			}
		}

		resp, body, err := g.doGet(ctx, endpoint)
		if err != nil {
			lastErr = err // 网络层错误，视为瞬态。
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("tools: 解析 GitHub 响应失败: %w", err)
			}
			return nil
		}

		gerr := newGitHubHTTPError(resp, body)
		lastErr = gerr
		if !gerr.retryable() {
			return gerr
		}
	}
	return fmt.Errorf("tools: 重试 %d 次后仍失败: %w", g.maxRetries, lastErr)
}

// doGet 执行一次原始 GET 请求，读取并关闭响应体。
// 返回原始 status 与 body，不做任何业务层解释。
//
// 入参：
//   - ctx：请求上下文（经 http.NewRequestWithContext 绑定到本次请求）。
//   - endpoint：完整的请求 URL。
//
// 返回值：
//   - *http.Response：原始响应（body 已读取并关闭，可读取其状态码与头）。
//   - []byte：响应体原始字节。
//   - error：构造请求、发送或读取响应体失败时返回（均属网络层错误，视为瞬态）。
func (g *GitHubTool) doGet(ctx context.Context, endpoint string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("tools: 构造请求失败: %w", err)
	}
	req.Header.Set("Accept", githubAccept)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("tools: 发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("tools: 读取响应失败: %w", err)
	}
	return resp, body, nil
}

// waitBeforeRetry 计算下一次重试前应等待的时长。
// 若上次错误携带 Retry-After（限流），优先按其等待；否则走指数退避。
//
// 入参：
//   - lastErr：上一次尝试的错误（可能为 *githubHTTPError，也可能为网络错误）。
//   - attempt：本次重试的序号（从 1 开始）。
//
// 返回值：
//   - time.Duration：本次重试前应等待的时长。
func (g *GitHubTool) waitBeforeRetry(lastErr error, attempt int) time.Duration {
	var gerr *githubHTTPError
	if errors.As(lastErr, &gerr) && gerr.RetryAfter > 0 {
		return gerr.RetryAfter
	}
	return g.backoff(attempt)
}

// —— 以下是 GitHub API 的响应载荷定义（只声明用到的字段）——
//
// Go 的 json 反序列化会忽略未声明字段，故无需穷举 API 全部字段。

// searchRepositoriesResponse 是 Search API 的响应体。
type searchRepositoriesResponse struct {
	Items []repository `json:"items"`
}

// repository 是 GitHub 仓库对象的「最小投影」。
// Search API 的 item 与 Repo API 的 detail 字段命名一致，故可共用同一个映射函数 mapRepo。
type repository struct {
	FullName        string    `json:"full_name"`        // "owner/name"，作为去重主键
	Name            string    `json:"name"`             // 仓库名（不含 owner）
	Owner           repoOwner `json:"owner"`            // 所有者
	Description     string    `json:"description"`      // 项目描述（不可信输入，喂模型前需防注入）
	Language        string    `json:"language"`         // 主编程语言，可能为 null
	StargazersCount int       `json:"stargazers_count"` // star 数（热度指标）
	HTMLURL         string    `json:"html_url"`         // 仓库主页
	Topics          []string  `json:"topics"`           // 主题标签
}

// repoOwner 是仓库所有者的最小投影。
type repoOwner struct {
	Login string `json:"login"`
}

// mapRepo 把 GitHub 返回的仓库对象映射为内部 schemas.Repo。
//
// 入参：
//   - r：GitHub 返回的仓库对象（search item 或 repo detail，两者字段一致）。
//
// 返回值：
//   - schemas.Repo：系统内部的统一仓库模型（以 FullName 作为去重主键）。
func mapRepo(r repository) schemas.Repo {
	return schemas.Repo{
		ID:          r.FullName, // "owner/name"，贯穿采集/去重/推送的主键
		Owner:       r.Owner.Login,
		Name:        r.Name,
		Description: r.Description,
		Language:    r.Language,
		Stars:       r.StargazersCount,
		URL:         r.HTMLURL,
		Topics:      r.Topics,
	}
}

// githubHTTPError 表示 GitHub 返回的非 2xx 错误。
type githubHTTPError struct {
	// StatusCode 是 HTTP 状态码。
	StatusCode int
	// Message 是 GitHub 返回的错误信息。
	Message string
	// RetryAfter 是 GitHub 建议的重试等待时长（来自 Retry-After 头，单位秒）；0 表示未提供。
	RetryAfter time.Duration
}

// Error 实现 error 接口，返回人类可读的错误描述。
//
// 入参：无。
//
// 返回值：
//   - string：形如 "github API 错误 (HTTP <状态码>): <信息>" 的文本。
func (e *githubHTTPError) Error() string {
	return fmt.Sprintf("github API 错误 (HTTP %d): %s", e.StatusCode, e.Message)
}

// retryable 判断该错误是否值得重试。
//
// 入参：无。
//
// 返回值：
//   - bool：true 表示瞬态、重试有望成功；false 表示重试只会得到同样的错。
func (e *githubHTTPError) retryable() bool {
	switch {
	case e.StatusCode == http.StatusTooManyRequests: // 429 限流
		return true
	case e.StatusCode >= 500 && e.StatusCode <= 599: // 5xx 服务器错误
		return true
	case e.StatusCode == http.StatusForbidden && e.RetryAfter > 0: // 403 + 限流（额度耗尽）
		return true
	default:
		return false
	}
}

// newGitHubHTTPError 由原始响应构造 githubHTTPError，并解析 Retry-After 头。
//
// 入参：
//   - resp：原始 HTTP 响应（只读其状态码与头）。
//   - body：响应体原始字节。
//
// 返回值：
//   - *githubHTTPError：含状态码、错误信息（解析失败则用原始文本）与重试等待时长的错误。
func newGitHubHTTPError(resp *http.Response, body []byte) *githubHTTPError {
	var env struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &env)
	if env.Message == "" {
		env.Message = strings.TrimSpace(string(body))
	}

	var retryAfter time.Duration
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			retryAfter = time.Duration(secs) * time.Second
		}
	}

	return &githubHTTPError{
		StatusCode: resp.StatusCode,
		Message:    env.Message,
		RetryAfter: retryAfter,
	}
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
