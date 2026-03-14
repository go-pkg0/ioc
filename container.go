package ioc

import "context"

// Factory 服务工厂函数（容器内部使用 any 存储异构类型）。
// 推荐使用泛型函数 Singleton[T] / Bind[T] 注册，编译期约束返回类型。
// ctx 由 Make 调用方传入，用于控制超时、取消和链路追踪。
type Factory func(ctx context.Context, c Container) (any, error)

// Container 是核心依赖注入容器接口。
//
// 实现必须是并发安全的。
//
// 推荐通过泛型函数操作容器，获得编译期类型安全：
//
//	ioc.Singleton(c, "db", func(ctx context.Context, c ioc.Container) (*sql.DB, error) { ... })
//	ioc.Decorate(c, "db", func(ctx context.Context, db *sql.DB, c ioc.Container) (*sql.DB, error) { ... })
//	db, err := ioc.Make[*sql.DB](ctx, c, "db")
//
// Container 接口方法为底层 API，仅在框架扩展或中间件场景中直接使用。
//
// Container 提供六类能力：
//   - 绑定：注册服务工厂（瞬时、单例、实例、别名）
//   - 解析：按名称获取服务实例（自动解析别名、检测循环依赖）
//   - AOP：装饰器和全局中间件，支持横切关注点无侵入注入
//   - 生命周期：优雅关闭和健康检查
//   - 管理：删除、内省、清空
//   - 安全：循环依赖检测、关闭状态保护
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

	// Alias 为已注册的服务创建别名。
	// Make("alias") 等价于 Make("abstract")。
	// 支持链式别名（最大深度 10 层）。
	Alias(alias, abstract string)

	// ---- 解析 ----

	// Make 按名称解析服务实例。
	// 自动解析别名，检测循环依赖（同 goroutine 内）。
	// 容器关闭后返回 ErrContainerClosed。
	// ctx 会传递给工厂函数和装饰器，用于超时控制和链路追踪。
	Make(ctx context.Context, abstract string) (any, error)

	// MustMake 解析服务实例，失败时 panic。
	// 适用于启动期和测试场景。
	MustMake(ctx context.Context, abstract string) any

	// Has 判断指定名称是否已注册绑定（自动解析别名）。
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
	// Close 是幂等的，重复调用返回 nil。
	// Close 后 Make 调用将返回 ErrContainerClosed。
	Close(ctx context.Context) error

	// HealthCheck 对所有实现了 HealthChecker 接口的单例并行执行健康检查。
	// 返回 name → error 映射，error 为 nil 表示健康。
	HealthCheck(ctx context.Context) map[string]error

	// ---- 管理 ----

	// Remove 删除指定名称的绑定、别名及其缓存的单例。
	// 适用于测试和热重载场景。
	Remove(abstract string)

	// Bindings 返回所有已注册名称的快照。
	// 适用于内省和调试。
	Bindings() []string

	// Flush 清空所有绑定、单例缓存、别名和装饰器。
	// 适用于测试场景。
	Flush()
}
