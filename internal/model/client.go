// Package model 抽象大模型调用能力。
//
// 用接口隔离具体厂商（Anthropic / OpenAI / 本地模型），
// 使上层（Agent、工具）只面向接口编程，不依赖任何具体实现——
// 从而可切换、可降级、可用 Mock 做单元测试。
package model

import "context"

// Client 是大模型调用的统一抽象。
//
// 阶段一先定义接口 + 占位，具体实现（如 Anthropic 的 HTTP 客户端）
// 在「阶段一：大模型调用基础」中补全。
type Client interface {
	// Complete 发起一次「非结构化」文本补全。
	//
	// 用于自由生成场景（如兜底降级时的人性化描述）。
	// 入参 prompt 为完整提示词；返回模型生成的文本。
	Complete(ctx context.Context, prompt string) (string, error)

	// CompleteJSON 发起一次「结构化」补全。
	//
	// 这是企业级的关键能力：要求模型输出符合约定 JSON Schema 的 JSON，
	// 并将结果反序列化到 result（必须传入指向 schemas 包结构体的指针）。
	// 模型输出非法 JSON / 结构不符时，必须返回 error，由上层执行降级（重试 → 简化 → 人工）。
	CompleteJSON(ctx context.Context, prompt string, result any) error
}
