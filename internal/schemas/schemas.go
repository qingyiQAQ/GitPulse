// Package schemas 定义系统内所有「结构化数据模型」。
//
// 企业级 Agent 的第一个硬要求：跨模块、跨网络、跨模型边界的数据，
// 都必须有明确的类型与 JSON 约束，确保下游可解析、可校验、可审计。
// 这里用 Go struct + json tag 承担 Python 生态中 Pydantic 的职责。
package schemas

// Repo 表示一个 GitHub 开源项目的最小元数据集合。
// 这是整个 Pipeline 的「数据原语」，贯穿采集、判断、摘要、推送四个环节。
type Repo struct {
	ID          string   `json:"id"`          // 唯一标识（owner/name），作为去重主键
	Owner       string   `json:"owner"`       // 仓库所有者
	Name        string   `json:"name"`        // 仓库名
	Description string   `json:"description"` // 项目描述（来自 GitHub，属「不可信输入」，喂给模型前需做注入防护）
	Language    string   `json:"language"`    // 主编程语言
	Stars       int      `json:"stars"`       // Star 数（热度指标之一）
	URL         string   `json:"url"`         // 仓库主页地址
	Topics      []string `json:"topics"`      // 主题标签列表
}

// Category 表示项目所属领域的分类。
// 用类型别名 + 常量集合约束取值，避免自由字符串带来的拼写不一致。
type Category string

const (
	CategoryInternet    Category = "internet"    // 互联网 / 网络
	CategoryProgramming Category = "programming" // 编程 / 开发工具
	CategoryAI          Category = "ai"          // 人工智能
	CategoryOther       Category = "other"       // 其他（判定为不相关）
)

// RelevanceVerdict 表示「相关性判断」的结构化结果。
//
// 这是模型输出的「契约」：必须可解析、可校验，而非自由文本。
// 其中的 Confidence 与 Reason 直接支撑可观测性（回答「为什么这么判」）与治理（低置信度人工复核）。
type RelevanceVerdict struct {
	IsRelevant bool     `json:"is_relevant"` // 是否与互联网/编程/AI 相关
	Confidence float64  `json:"confidence"`  // 置信度 0~1；低于阈值进入人工复核
	Category   Category `json:"category"`    // 所属分类
	Reason     string   `json:"reason"`      // 可解释的判据
}

// Summary 表示对相关项目生成的结构化摘要。
type Summary struct {
	Title     string   `json:"title"`      // 一句话标题
	OneLiner  string   `json:"one_liner"`  // 一句话概述
	KeyPoints []string `json:"key_points"` // 核心要点列表
}

// Notification 表示一条最终待推送的消息，聚合了前序步骤的全部产物。
// 它是 Pipeline 的「输出契约」，也是推送工具的「输入契约」。
type Notification struct {
	Repo    Repo             `json:"repo"`    // 项目元数据
	Verdict RelevanceVerdict `json:"verdict"` // 相关性判断结果
	Summary Summary          `json:"summary"` // 摘要
	Channel string           `json:"channel"` // 目标推送渠道
}
