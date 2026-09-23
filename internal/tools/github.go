package tools

import (
	"context"
	"encoding/json"

	"github.com/gitpulse/gitpulse/internal/schemas"
)

// GitHubTool 封装 GitHub 数据采集能力（Pipeline 第一步「采集」）。
//
// 子能力：
//   - fetchTrending：抓取热门项目列表
//   - fetchDetail：  抓取单个项目的详情
//
// 工程重点：
//   - GitHub Trending 无官方 API，阶段一可先用 Search API（按 stars/created 排序）近似；
//   - 限流（Retry-After 退避）与幂等是这里的核心可靠性问题。
type GitHubTool struct {
	token string // GitHub 访问令牌（可选，提升限流额度）
}

// NewGitHubTool 构造 GitHub 采集工具。
func NewGitHubTool(token string) *GitHubTool {
	return &GitHubTool{token: token}
}

// Name 实现 Tool 接口。
func (g *GitHubTool) Name() string { return "github" }

// Description 实现 Tool 接口。
func (g *GitHubTool) Description() string {
	// TODO(阶段一): 返回喂给模型的功能描述（说明支持 trending / detail 两个子能力）。
	return ""
}

// Run 实现 Tool 接口：按 args 中的 action 分发到具体子能力。
func (g *GitHubTool) Run(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	// TODO(阶段一):
	//   1. 解析 args，取出 action（"trending" | "detail"）及对应参数
	//   2. 分发到 fetchTrending / fetchDetail
	//   3. 将结果序列化为 JSON 返回
	return nil, nil
}

// fetchTrending 抓取热门项目列表。
func (g *GitHubTool) fetchTrending(ctx context.Context, language string, limit int) ([]schemas.Repo, error) {
	// TODO(阶段一):
	//   1. 调用 GitHub Search API（按 stars/created 排序）近似 trending
	//   2. 处理限流：读取 Retry-After，指数退避重试
	//   3. 映射为 []schemas.Repo
	return nil, nil
}

// fetchDetail 抓取单个项目的详情（README、完整描述、topics 等）。
func (g *GitHubTool) fetchDetail(ctx context.Context, owner, name string) (*schemas.Repo, error) {
	// TODO(阶段一): 调用 repo API 补齐元数据。
	return nil, nil
}
