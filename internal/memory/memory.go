// Package memory 实现 Agent 的记忆层。
//
// 阶段一聚焦两类记忆：
//  1. 去重记忆：记录「已处理」的仓库，保证幂等，避免重复推送；
//  2. 偏好记忆：沉淀用户兴趣标签，反向优化相关性判断（后续阶段）。
//
// 通过接口抽象存储实现：MVP 用内存，后续替换 SQLite / PostgreSQL 而不改上层代码。
package memory

import "context"

// Store 定义记忆层的抽象能力。
//
// 之所以抽接口：记忆的「语义」稳定（去重、偏好），但「存储介质」会演进
// （内存 → SQLite → PostgreSQL/Redis）。上层（Agent/Pipeline）只依赖语义，不依赖介质。
type Store interface {
	// MarkSeen 记录某个仓库已被处理。
	// repoID 为仓库唯一标识（schemas.Repo.ID）。
	MarkSeen(ctx context.Context, repoID string) error

	// IsSeen 判断仓库是否已处理过，用于去重。
	IsSeen(ctx context.Context, repoID string) (bool, error)
}

// InMemoryStore 是 Store 的内存实现，仅用于 MVP 与单元测试。
// 数据不持久化，进程退出即丢失；生产必须替换为持久化实现。
type InMemoryStore struct {
	// TODO(阶段一): 用 map[string]struct{} 保存已处理集合 + sync.RWMutex 保证并发安全。
}

// NewInMemoryStore 构造内存实现。
func NewInMemoryStore() *InMemoryStore {
	// TODO(阶段一): 初始化内部集合与锁。
	return &InMemoryStore{}
}

// MarkSeen 实现 Store 接口：写入已处理集合。
func (s *InMemoryStore) MarkSeen(ctx context.Context, repoID string) error {
	// TODO(阶段一): 加写锁，写入集合；返回 nil。
	return nil
}

// IsSeen 实现 Store 接口：判断是否已处理。
func (s *InMemoryStore) IsSeen(ctx context.Context, repoID string) (bool, error) {
	// TODO(阶段一): 加读锁，判断集合是否包含 repoID。
	return false, nil
}
