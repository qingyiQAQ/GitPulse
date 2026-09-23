package tools

import (
	"context"
	"encoding/json"

	"github.com/gitpulse/gitpulse/internal/push"
	"github.com/gitpulse/gitpulse/internal/schemas"
)

// PushTool 负责「推送」（Pipeline 最后一步）：将最终结果通过渠道适配器发送给用户。
//
// 这是「高风险动作」（向外发送信息），因此在治理上需要：
//   - 低置信度的结果在推送前由上层拦截进入人工复核；
//   - 每次推送落审计日志（谁、何时、推了什么）。
type PushTool struct {
	adapter push.Adapter // 推送渠道适配器（注入，便于替换与 Mock）
}

// NewPushTool 构造推送工具。
func NewPushTool(adapter push.Adapter) *PushTool {
	return &PushTool{adapter: adapter}
}

// Name 实现 Tool 接口。
func (p *PushTool) Name() string { return "push" }

// Description 实现 Tool 接口。
func (p *PushTool) Description() string {
	// TODO(阶段一): 返回喂给模型的功能描述。
	return ""
}

// Run 实现 Tool 接口：推送一条通知。
func (p *PushTool) Run(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	// TODO(阶段一):
	//   1. 解析 args 为 schemas.Notification
	//   2. 调用 p.send 完成推送
	return nil, nil
}

// send 执行一次推送（强类型入口，供 Run 内部与单元测试使用）。
func (p *PushTool) send(ctx context.Context, n schemas.Notification) error {
	// TODO(阶段一):
	//   1. 将 schemas.Notification 转为 push.Message
	//   2. 调用 p.adapter.Send
	//   3. 失败按指数退避重试；最终失败返回 error 交由上层告警
	return nil
}
