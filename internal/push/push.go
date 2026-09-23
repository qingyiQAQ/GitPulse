// Package push 抽象「推送渠道」。
//
// 通过统一接口屏蔽微信 / 邮件 / 钉钉 / Slack 的差异（Adapter 模式），
// 使上层只需面向 Adapter 编程，新增渠道不触碰业务逻辑。
package push

import "context"

// Message 是一条经过标准化的待推送消息。
// 上层（推送工具）把业务产物（schemas.Notification）转成此标准结构，再交给具体渠道。
type Message struct {
	Title   string // 标题
	Content string // 正文
	Channel string // 目标渠道标识
}

// Adapter 定义推送渠道的抽象。
type Adapter interface {
	// Send 将一条消息推送到目标渠道。
	// 失败应返回 error，由上层决定重试（指数退避）或告警——推送本身不负责重试策略。
	Send(ctx context.Context, msg Message) error
}

// ConsoleAdapter 是 Adapter 的控制台实现，用于本地验证与测试。
// 它将消息打印到标准输出，是「先跑通链路」的最小实现。
type ConsoleAdapter struct{}

// NewConsoleAdapter 构造控制台适配器。
func NewConsoleAdapter() *ConsoleAdapter {
	return &ConsoleAdapter{}
}

// Send 实现 Adapter 接口：打印消息而非真正发送。
func (c *ConsoleAdapter) Send(ctx context.Context, msg Message) error {
	// TODO(阶段一): 用 log/slog 打印 msg.Title 与 msg.Content，便于本地观察推送结果。
	return nil
}
