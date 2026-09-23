// Package prompt 管理 Prompt 资产。
//
// 企业级视角：Prompt 不是散落在代码里的字符串，而是「资产」——
// 需要命名、版本化、可追溯、可回滚。
// 阶段一先用内存中的集中定义承载这一理念，后续迁移到文件（prompts/*.md）+ 版本控制。
package prompt

// Manager 是 Prompt 资产的集中访问入口。
//
// 上层（工具）通过「名称」获取 Prompt，而非硬编码字符串，
// 从而统一管理与审计：谁在什么场景用了哪份 Prompt，一目了然。
type Manager struct {
	// templates 保存「名称 → 模板」的映射。
	// 字段私有：外部只能通过 Get 读取，保证 Prompt 的访问可被追踪、可被统一替换。
	templates map[string]string
}

// NewManager 构造一个 Prompt 管理器。
//
// 阶段一由调用方注册内置模板（相关性判断、摘要生成的 system / user 模板）；
// 后续改为从目录加载并携带版本号，实现可回滚。
func NewManager() *Manager {
	// TODO(阶段一): 初始化 templates，注册内置 Prompt 模板。
	return &Manager{}
}

// Get 返回指定名称的 Prompt 模板。
//
// 未注册的名称必须返回 error，避免「静默拿到空字符串」导致模型收到残缺指令——
// 这是结构化输出之外的又一层「失败要显式」的护栏。
func (m *Manager) Get(name string) (string, error) {
	// TODO(阶段一): 从 templates 查找；不存在则返回含名称的错误。
	return "", nil
}
