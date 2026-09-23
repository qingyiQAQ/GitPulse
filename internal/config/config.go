// Package config 负责应用配置的加载与校验。
//
// 企业级原则：配置与代码分离；密钥一律通过环境变量注入，绝不硬编码、绝不入库。
// 阶段一使用标准库 os.Getenv 做最简实现，后续可替换为 caarlos0/env 或 koanf 以支持文件/远程配置。
package config

// Config 汇总了应用运行所需的全部配置项。
//
// 字段即「契约」：任何新增可配置项都应显式声明在此结构体中，
// 而不是散落在各包的代码里通过 os.Getenv 各取所需——集中管理才能保证一致性与可审计。
type Config struct {
	// AnthropicAPIKey 是 LLM 服务（Anthropic）的访问密钥。
	// 必填：缺失时 Load 必须返回错误，拒绝以残缺配置启动。
	AnthropicAPIKey string

	// GitHubToken 是 GitHub API 的访问令牌。
	// 可选：未配置时仍可用匿名访问，但限流额度更低、抓取更容易被限。
	GitHubToken string

	// PushChannel 指定默认推送渠道（console / wechat / email / dingtalk / slack）。
	// 默认 console（本地验证用）。
	PushChannel string

	// RelevanceThreshold 是相关性判断的置信度阈值（0~1）。
	// 模型输出的 confidence 低于该值时，判定结果进入人工复核而非直接推送。
	// 这是「治理」维度的护栏：高风险动作（对外推送）不盲目信任模型。
	RelevanceThreshold float64
}

// Load 从环境变量读取并校验配置。
//
// 返回值契约：返回的 *Config 一定是「可用」的——
// 任一必填项缺失或非法，直接返回 error，绝不返回半成品配置。
func Load() (*Config, error) {
	// TODO(阶段一): 实现真正的读取与校验。
	//   1. 读取 ANTHROPIC_API_KEY（必填）、GITHUB_TOKEN、PUSH_CHANNEL、RELEVANCE_THRESHOLD
	//   2. 解析 RELEVANCE_THRESHOLD 为 float64，非法值报错
	//   3. 校验必填项，缺失时返回含字段名的明确错误（便于定位）
	//   4. 设置默认值：PushChannel 默认 "console"、阈值默认 0.7
	return &Config{}, nil
}
