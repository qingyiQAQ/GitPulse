package tools

import (
	"context"
	"encoding/json"

	"github.com/gitpulse/gitpulse/internal/model"
	"github.com/gitpulse/gitpulse/internal/prompt"
	"github.com/gitpulse/gitpulse/internal/schemas"
)

// SummarizeTool 负责「摘要生成」（Pipeline 第三步）：
// 对判定为相关的项目生成结构化摘要。
//
// 与 JudgeTool 一样依赖结构化输出：结果必须符合 schemas.Summary 的契约，
// 长度受控（标题 + 一句话 + 要点），便于推送给用户时保持精炼。
type SummarizeTool struct {
	client  model.Client    // LLM 客户端（注入）
	prompts *prompt.Manager // Prompt 管理器（注入）
}

// NewSummarizeTool 构造摘要生成工具。
func NewSummarizeTool(client model.Client, prompts *prompt.Manager) *SummarizeTool {
	return &SummarizeTool{client: client, prompts: prompts}
}

// Name 实现 Tool 接口。
func (s *SummarizeTool) Name() string { return "summarize_repo" }

// Description 实现 Tool 接口。
func (s *SummarizeTool) Description() string {
	// TODO(阶段一): 返回喂给模型的功能描述。
	return ""
}

// Run 实现 Tool 接口：对输入项目生成摘要。
// 入参 args 为项目元数据（JSON）；返回结构化摘要（JSON）。
func (s *SummarizeTool) Run(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	// TODO(阶段一):
	//   1. 解析 args 为 schemas.Repo（必要时含 RelevanceVerdict）
	//   2. 调用 s.summarize 生成摘要
	//   3. 序列化为 JSON 返回
	return nil, nil
}

// summarize 执行一次摘要生成，返回强类型结果（供 Run 内部与单元测试使用）。
func (s *SummarizeTool) summarize(ctx context.Context, repo schemas.Repo, verdict schemas.RelevanceVerdict) (schemas.Summary, error) {
	// TODO(阶段一):
	//   1. 从 s.prompts 取摘要模板，拼接 repo 元数据与判断理由（verdict.Reason）
	//   2. 调用 s.client.CompleteJSON 反序列化为 schemas.Summary
	//   3. 控制输出长度，失败走降级策略
	return schemas.Summary{}, nil
}
