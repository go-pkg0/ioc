package ioc

// Factory 服务工厂函数。
// 接收容器引用，返回服务实例或错误。
type Factory func(Container) (any, error)

// Container 是核心依赖注入容器接口。
//
// 实现必须是并发安全的。
//
// Container 提供三类能力：
//   - 绑定：注册服务工厂（瞬时、单例、实例）
//   - 解析：按名称获取服务实例
//   - AOP：装饰器和全局中间件，支持横切关注点无侵入注入
type Container interface {
	// ---- 绑定 ----

	// Bind 注册瞬时工厂，每次 Make 调用都会创建新实例。
	Bind(abstract string, factory Factory)

	// Singleton 注册单例工厂，首次 Make 后缓存结果，后续调用直接返回缓存。
	Singleton(abstract string, factory Factory)

	// Instance 将已构建的实例直接注册为单例。
	Instance(abstract string, value any)

	// ---- 解析 ----

	// Make 按名称解析服务实例。
	// 未找到绑定时返回包装了 ErrNotBound 的错误。
	Make(abstract string) (any, error)

	// MustMake 解析服务实例，失败时 panic。
	// 适用于启动期和测试场景。
	MustMake(abstract string) any

	// Has 判断指定名称是否已注册绑定。
	Has(abstract string) bool

	// ---- AOP ----

	// Decorate 为指定服务添加装饰器。
	// 装饰器在工厂函数返回后、缓存前执行，可包装/替换/增强实例。
	// 多个装饰器按注册顺序依次执行（管道模型）。
	Decorate(abstract string, decorator Decorator)

	// Use 注册全局中间件，对所有 Make 调用的首次解析生效。
	// 中间件按注册顺序形成洋葱模型。
	Use(middleware ...Middleware)

	// ---- 管理 ----

	// Remove 删除指定名称的绑定及其缓存的单例。
	// 适用于测试和热重载场景。
	Remove(abstract string)

	// Bindings 返回所有已注册名称的快照。
	// 适用于内省和调试。
	Bindings() []string

	// Flush 清空所有绑定、单例缓存和装饰器。
	// 适用于测试场景。
	Flush()
}
