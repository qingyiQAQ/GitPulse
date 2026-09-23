package tools

import (
	"context"
	"encoding/json"

	"github.com/gitpulse/gitpulse/internal/model"
	"github.com/gitpulse/gitpulse/internal/prompt"
	"github.com/gitpulse/gitpulse/internal/schemas"
)

// JudgeTool 负责「相关性判断」（Pipeline 第二步）：
// 调用 LLM 判断项目是否与互联网 / 编程 / AI 相关。
//
// 这是结构化输出的关键示范：模型输出必须符合 schemas.RelevanceVerdict 的 JSON 契约，
// 可解析、可校验；解析失败走三级降级（重试 → 简化 → 人工）。
type JudgeTool struct {
	client  model.Client    // LLM 客户端（注入，便于替换与 Mock 测试）
	prompts *prompt.Manager // Prompt 管理器（注入，Prompt 版本化）
}

// NewJudgeTool 构造相关性判断工具。
func NewJudgeTool(client model.Client, prompts *prompt.Manager) *JudgeTool {
	return &JudgeTool{client: client, prompts: prompts}
}

// Name 实现 Tool 接口。
func (j *JudgeTool) Name() string { return "judge_relevance" }

// Description 实现 Tool 接口。
func (j *JudgeTool) Description() string {
	// TODO(阶段一): 返回喂给模型的功能描述。
	return ""
}

// Run 实现 Tool 接口：对输入项目做相关性判断。
// 入参 args 为一个或多个项目的元数据（JSON）；返回结构化判定结果（JSON）。
func (j *JudgeTool) Run(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	// TODO(阶段一):
	//   1. 解析 args 为 []schemas.Repo（或单个 Repo）
	//   2. 逐个调用 j.judge 完成判断
	//   3. 将结果集合序列化为 JSON 返回
	return nil, nil
}

// judge 执行一次判断，返回强类型结果（供 Run 内部与单元测试使用）。
//
// 强类型入口让测试可以直接断言 RelevanceVerdict 的字段，而不必处理 JSON。
func (j *JudgeTool) judge(ctx context.Context, repo schemas.Repo) (schemas.RelevanceVerdict, error) {
	// TODO(阶段一):
	//   1. 从 j.prompts 取相关性判断模板，拼接 repo 元数据
	//      （注意：repo.Description 是「不可信输入」，需做注入防护 / 边界隔离）
	//   2. 调用 j.client.CompleteJSON 反序列化为 schemas.RelevanceVerdict
	//   3. 校验置信度范围 [0,1]；解析失败走降级策略
	return schemas.RelevanceVerdict{}, nil
}
