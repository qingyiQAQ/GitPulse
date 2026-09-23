// Package main 是 GitPulse 的进程入口。
//
// 职责边界：main 只负责「装配」（依赖注入），不负责「实现」。
// 所有业务逻辑一律下沉到 internal/ 各包，main 保持精简、可读、可测试。
package main

import (
	"log/slog"
	"os"
)

func main() {
	// 阶段一待完成的装配流程（当前为占位实现，仅打通可编译骨架）：
	//
	//   1. 加载配置        —— config.Load()，读取并校验环境变量
	//   2. 初始化日志       —— slog 使用 JSON handler，后续接入 OTLP / 可观测性平台
	//   3. 构造组件        —— model.Client、tools.Registry（注册 github/judge/summarize/push 四个工具）、
	//                          memory.Store、push.Adapter
	//   4. 组装编排        —— agent.NewPipeline（阶段一确定性链）或 agent.New（自由循环）
	//   5. 执行一次任务     —— pipeline.Run(context.Background())，失败以非零退出码返回
	//
	// 企业级要求：main 中的每一步装配失败都应尽早、显式地失败（fail fast），
	// 而非带着残缺依赖继续运行。

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	slog.Info("gitpulse 启动（阶段一占位实现，装配逻辑见 main 注释 TODO）")
}
