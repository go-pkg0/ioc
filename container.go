package ioc

import "context"

// Factory 服务工厂函数。
// 接收 context 和容器引用，返回服务实例或错误。
// context 由 Make 调用方传入，用于控制超时、取消和链路追踪。
type Factory func(ctx context.Context, c Container) (any, error)

// Container 是核心依赖注入容器接口。
//
// 实现必须是并发安全的。
//
// Container 提供五类能力：
//   - 绑定：注册服务工厂（瞬时、单例、实例）
//   - 解析：按名称获取服务实例
//   - AOP：装饰器和全局中间件，支持横切关注点无侵入注入
//   - 生命周期：优雅关闭和健康检查
//   - 管理：删除、内省、清空
type Container interface {
	// ---- 绑定 ----

	// Bind 注册瞬时工厂，每次 Make 调用都会创建新实例。
	Bind(abstract string, factory Factory)

	// Singleton 注册单例工厂，首次 Make 后缓存结果，后续调用直接返回缓存。
	// 若工厂返回错误，错误将被缓存，后续调用返回相同错误。
	// 如需重试，请先调用 Remove 再重新注册。
	Singleton(abstract string, factory Factory)

	// Instance 将已构建的实例直接注册为单例。
	// 注意：Instance 注册的服务不经过装饰器和中间件链。
	Instance(abstract string, value any)

	// ---- 解析 ----

	// Make 按名称解析服务实例。
	// ctx 会传递给工厂函数和装饰器，用于超时控制和链路追踪。
	// 未找到绑定时返回包装了 ErrNotBound 的错误。
	Make(ctx context.Context, abstract string) (any, error)

	// MustMake 解析服务实例，失败时 panic。
	// 适用于启动期和测试场景。
	MustMake(ctx context.Context, abstract string) any

	// Has 判断指定名称是否已注册绑定。
	Has(abstract string) bool

	// ---- AOP ----

	// Decorate 为指定服务添加装饰器。
	// 装饰器在工厂函数返回后、缓存前执行，可包装/替换/增强实例。
	// 多个装饰器按注册顺序依次执行（管道模型）。
	// 注意：对 Instance 注册的服务无效（已直接缓存）。
	Decorate(abstract string, decorator Decorator)

	// Use 注册全局中间件，对所有 Make 调用的首次解析生效。
	// 中间件按注册顺序形成洋葱模型。
	Use(middleware ...Middleware)

	// ---- 生命周期 ----

	// Close 优雅关闭所有实现了 Closeable 接口的单例。
	// 按创建的逆序执行 Close，确保依赖关系正确释放。
	// 所有错误通过 errors.Join 聚合返回。
	Close(ctx context.Context) error

	// HealthCheck 对所有实现了 HealthChecker 接口的单例执行健康检查。
	// 返回 name → error 映射，error 为 nil 表示健康。
	HealthCheck(ctx context.Context) map[string]error

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
