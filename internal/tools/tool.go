// Package tools 是 GitPulse 的「工具层」。
//
// 工具是 Agent 从「能说」到「能干活」的关键一步（对应 Function Calling）：
// 每个工具封装一个外部系统或一个模型能力，具备清晰的名称、描述与 JSON 参数契约，
// 可被 Agent 统一发现、调用与追踪。
//
// 设计原则：工具是「薄封装」，只负责与外部系统交互；
// 业务逻辑（阈值、去重、降级）放在编排层（internal/agent），而非塞进工具内部——
// 这样工具才可复用、可替换、可独立测试。
package tools

import (
	"context"
	"encoding/json"
)

// Tool 定义了一个可被 Agent 调用的工具。
type Tool interface {
	// Name 返回工具的唯一标识，用于注册表索引与 trace 标记。
	Name() string

	// Description 返回工具的功能描述。
	// 这份描述会喂给模型（function calling 的 description），决定模型何时、如何调用它。
	Description() string

	// Run 执行工具。
	//
	// 入参 args 为 JSON 编码的参数（与 LLM function calling 的 arguments 对齐）；
	// 返回值为 JSON 编码的结果。任一步失败都应返回 error，由上层决定重试或降级。
	// 使用 json.RawMessage 而非具体结构体，是为了让「通用执行循环」无需知道每个工具的参数细节。
	Run(ctx context.Context, args json.RawMessage) (json.RawMessage, error)
}

// Registry 是工具注册表，Agent 通过它发现并调度工具。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 构造空的注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个工具。
// 同名工具重复注册视为错误（保证工具标识全局唯一，便于 trace 与审计）。
func (r *Registry) Register(t Tool) error {
	// TODO(阶段一): 校验名称非空、不重复，写入 map；冲突返回 error。
	return nil
}

// Get 按名称取工具。未注册时返回 ok=false。
func (r *Registry) Get(name string) (Tool, bool) {
	// TODO(阶段一): 查 map。
	return nil, false
}

// All 返回全部已注册工具，供 Agent 生成工具清单（喂给模型）。
func (r *Registry) All() []Tool {
	// TODO(阶段一): 遍历 map，返回切片。
	return nil
}
