package agent

import (
	"context"

	"github.com/gitpulse/gitpulse/internal/memory"
	"github.com/gitpulse/gitpulse/internal/tools"
)

// Pipeline 是阶段一（M1）的「确定性执行链」。
//
// 执行顺序固定为：采集 → 相关性判断 → 摘要生成 → 推送。
// 之所以阶段一先做「确定性链」而非让模型自由规划，是因为（见 learn/learn.md 阶段三）：
//   - 阶段一目标是跑通闭环、验证结构化输出与工具调用；
//   - 确定性链可预测、易审计，适合作为第一步的「默认选择」；
//   - 自由规划（模型驱动的 Planner）留给阶段三的 DAG Plan & Execute。
//
// Pipeline 是 Agent 循环之上的一层「固定编排」：每一步仍是 PLAN→ACT→OBSERVE，
// 只是 PLAN 被固化为「按固定顺序执行」。
type Pipeline struct {
	registry *tools.Registry
	memory   memory.Store
	// threshold 是相关性置信度阈值：低于它则进入人工复核而非直接推送。
	threshold float64
}

// PipelineConfig 是 Pipeline 的构造参数。
type PipelineConfig struct {
	Registry  *tools.Registry // 工具注册表（须已注册 github / judge / summarize / push）
	Memory    memory.Store    // 记忆层（去重）
	Threshold float64         // 置信度阈值，0 表示使用默认值
}

// NewPipeline 构造确定性 Pipeline。
func NewPipeline(cfg PipelineConfig) *Pipeline {
	// TODO(阶段一): 校验依赖（Registry / Memory 非 nil），设置默认阈值。
	return &Pipeline{registry: cfg.Registry, memory: cfg.Memory, threshold: cfg.Threshold}
}

// Run 执行一次完整的「采集 → 判断 → 摘要 → 推送」闭环。
//
// 整体流程（每一步的详细职责见对应 TODO）：
//  1. 采集热门项目      —— 调用 github 工具，取回 []schemas.Repo
//  2. 去重             —— 通过 memory 过滤已处理项目（幂等）
//  3. 相关性判断        —— 对每个新项目调用 judge 工具，得到 RelevanceVerdict
//  4. 摘要生成          —— 对「相关」项目调用 summarize 工具，得到 Summary
//  5. 推送             —— 调用 push 工具；低置信度结果拦截进入人工复核
//  6. 更新记忆          —— 记录已处理项目
//
// 可靠性约束：
//   - 单条处理失败不中断整体：失败降级为「跳过 + 记日志 + 告警」；
//   - 全程埋 trace，保证可观测性（能回答「它为什么推了这个」）。
func (p *Pipeline) Run(ctx context.Context) error {
	// TODO(阶段一): 编排上述六步，串联四个工具。
	return nil
}
