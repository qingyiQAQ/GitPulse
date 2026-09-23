module github.com/gitpulse/gitpulse

// GitPulse：自动监控 GitHub 热门项目 → 判断相关性 → 生成摘要 → 推送的 AI Agent 项目。
//
// 说明：
//   - 模块路径使用占位域名 github.com/gitpulse/gitpulse，推送到真实仓库前请改为实际路径。
//   - 阶段一仅依赖 Go 标准库，不引入第三方依赖（保证开箱可编译），
//     后续阶段按需引入（OpenTelemetry、go-github、数据库驱动等）。
//
// 最低要求 Go 1.21+（使用了 log/slog），推荐 1.22+。
go 1.22
