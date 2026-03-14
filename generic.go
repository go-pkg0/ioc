package ioc

import (
	"context"
	"fmt"
)

// ---------------------------------------------------------------------------
// 泛型注册函数 — 编译期约束工厂返回类型，消除 any
// ---------------------------------------------------------------------------

// Singleton 注册类型安全的单例工厂。
// 工厂返回类型 T 由编译器推断并强制约束。
//
// 示例:
//
//	ioc.Singleton(c, "db", func(ctx context.Context, c ioc.Container) (*sql.DB, error) {
//	    return sql.Open("mysql", dsn)
//	})
func Singleton[T any](c Container, abstract string, factory func(ctx context.Context, c Container) (T, error)) {
	c.Singleton(abstract, func(ctx context.Context, c Container) (any, error) {
		return factory(ctx, c)
	})
}

// Bind 注册类型安全的瞬时工厂。
// 每次 Make 均调用工厂创建新实例。
//
// 示例:
//
//	ioc.Bind(c, "req", func(ctx context.Context, c ioc.Container) (*http.Request, error) {
//	    return http.NewRequestWithContext(ctx, "GET", url, nil)
//	})
func Bind[T any](c Container, abstract string, factory func(ctx context.Context, c Container) (T, error)) {
	c.Bind(abstract, func(ctx context.Context, c Container) (any, error) {
		return factory(ctx, c)
	})
}

// Instance 注册类型安全的预构建实例为单例。
//
// 示例:
//
//	ioc.Instance(c, "config", &AppConfig{Port: 8080})
func Instance[T any](c Container, abstract string, value T) {
	c.Instance(abstract, value)
}

// ---------------------------------------------------------------------------
// 泛型装饰器 — 入参和返回类型均为 T，无需手动断言
// ---------------------------------------------------------------------------

// Decorate 为指定服务添加类型安全的装饰器。
// 装饰器接收和返回具体类型 T，无需手动类型断言。
// 类型不匹配时返回 ErrTypeMismatch。
//
// 示例:
//
//	ioc.Decorate(c, "db", func(ctx context.Context, pool *ConnectionPool, c ioc.Container) (*ConnectionPool, error) {
//	    return NewMonitoredPool(pool), nil
//	})
func Decorate[T any](c Container, abstract string, fn func(ctx context.Context, instance T, c Container) (T, error)) {
	c.Decorate(abstract, func(ctx context.Context, instance any, c Container) (any, error) {
		typed, ok := instance.(T)
		if !ok {
			var zero T
			return nil, fmt.Errorf("%w: decorator for %q received %T, want %T", ErrTypeMismatch, abstract, instance, zero)
		}
		return fn(ctx, typed, c)
	})
}

// ---------------------------------------------------------------------------
// 泛型解析函数 — 返回具体类型 T，无需手动断言
// ---------------------------------------------------------------------------

// Make 解析服务实例并断言为目标类型 T。
// 解析失败返回底层错误，类型不匹配返回 ErrTypeMismatch。
//
// 示例:
//
//	mgr, err := ioc.Make[*log.Manager](ctx, c, "log")
func Make[T any](ctx context.Context, c Container, abstract string) (T, error) {
	val, err := c.Make(ctx, abstract)
	if err != nil {
		var zero T
		return zero, err
	}
	typed, ok := val.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%w: %q resolved to %T, want %T", ErrTypeMismatch, abstract, val, zero)
	}
	return typed, nil
}

// MustMake 解析并断言类型，失败时 panic。
// 适用于启动期和测试场景。
//
// 示例:
//
//	mgr := ioc.MustMake[*log.Manager](ctx, c, "log")
func MustMake[T any](ctx context.Context, c Container, abstract string) T {
	v, err := Make[T](ctx, c, abstract)
	if err != nil {
		panic(err)
	}
	return v
}
