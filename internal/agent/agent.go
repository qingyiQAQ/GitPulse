// Package agent 是 GitPulse 的编排层：实现 Agent 核心循环与任务编排。
//
// 阶段一聚焦「单 Agent 循环」——这是所有复杂架构的底层执行单元：
//
//	PLAN → ACT（调用工具）→ OBSERVE（读取结果）
//
// 后续阶段（三）会在此之上扩展为 DAG Plan & Execute 与多 Agent 编排，
// 但万变不离其宗：一切编排都是在这条循环上的组合与扩展。
package agent

import (
	"context"

	"github.com/gitpulse/gitpulse/internal/memory"
	"github.com/gitpulse/gitpulse/internal/model"
	"github.com/gitpulse/gitpulse/internal/tools"
)

// State 表示 Agent 在单次运行中的工作状态（会话级记忆）。
// 它承载 OBSERVE 阶段累积的上下文：当前目标、中间产物、已执行步骤。
type State struct {
	// Goal 是本次运行的目标描述（如「抓取今日热门项目并推送相关项目」）。
	Goal string

	// Steps 记录已执行的步骤数，用于限制循环上限（防止失控的护栏之一）。
	Steps int
}

// Config 是 Agent 的构造参数。
// 依赖全部通过构造函数注入，保证可测试性（测试可注入 Mock）。
type Config struct {
	// Registry 提供工具发现与调度能力。
	Registry *tools.Registry

	// Client 提供大模型调用能力（规划 / 分类 / 摘要）。
	Client model.Client

	// Memory 提供跨运行的持久记忆（去重、偏好）。
	Memory memory.Store

	// MaxSteps 限制单次运行的最大循环步数，防止死循环（护栏之一）。
	// 0 表示使用默认值。
	MaxSteps int
}

// Agent 是单 Agent 的核心。
// 它维护「思考—行动—观察」循环所需的所有依赖与状态。
type Agent struct {
	cfg Config
}

// New 构造 Agent。
func New(cfg Config) *Agent {
	// TODO(阶段一): 校验必填依赖（Registry / Client / Memory 非 nil），设置 MaxSteps 默认值。
	return &Agent{cfg: cfg}
}

// Step 执行一次「思考—行动—观察」单元。
//
// 这是所有 Agent 的底层执行单元，后续复杂架构都是在这条循环上做编排与扩展。
// 三个阶段语义：
//
//	PLAN    —— 决定下一步做什么（阶段一为确定性计划，阶段三改为模型规划）
//	ACT     —— 调用工具（通过 cfg.Registry），失败按指数退避重试
//	OBSERVE —— 读取工具返回，校验输出，写回 state，记录 trace
//
// 入参 state 为当前状态（指针，原地更新）；返回 error 表示本步失败（由 Run 决定是否中止）。
func (a *Agent) Step(ctx context.Context, state *State) error {
	// TODO(阶段一): 实现 PLAN / ACT / OBSERVE。
	//   1. PLAN:    从 state.Goal 推断下一步动作（阶段一可走确定性链）
	//   2. ACT:     从 cfg.Registry 取工具并调用，失败按指数退避重试
	//   3. OBSERVE: 校验工具输出，更新 state，记录 trace（可观测性）
	//   4. 步数自增，超过 cfg.MaxSteps 时返回终止信号
	return nil
}

// Run 驱动完整循环，直到任务完成或达到步数上限。
// 返回 error 用于进程退出码与告警。
func (a *Agent) Run(ctx context.Context) error {
	// TODO(阶段一):
	//   1. 初始化 State（Goal、Steps=0）
	//   2. 循环调用 Step，直至任务完成或超限
	//   3. 汇总运行轨迹，返回结果
	return nil
}
