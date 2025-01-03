package ioc

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// containerInternals 容器内部接口，用于 Application 访问单例信息。
// 自定义 Container 实现可选择实现此接口以支持 Shutdown/HealthCheck。
type containerInternals interface {
	Order() []string
	Singletons() map[string]any
}

// Application 编排 ServiceProvider 的生命周期。
//
// 不包含 HTTP 服务器、信号处理、配置加载等框架层逻辑，
// 这些由具体框架（如 q-gin）在上层实现。
//
// Application 的 Register/Boot 应在同一 goroutine 中顺序调用（启动期）。
// Shutdown 可在任意 goroutine 调用（关闭期）。
type Application struct {
	mu        sync.Mutex
	container Container
	providers []ServiceProvider
	booted    bool
}

// NewApp 创建 Application 实例。
// 若未通过 WithContainer 指定容器，则自动创建默认容器。
func NewApp(opts ...AppOption) *Application {
	a := &Application{}
	for _, o := range opts {
		o(a)
	}
	if a.container == nil {
		a.container = New()
	}
	return a
}

// Register 追加服务提供者。必须在 Boot 之前调用。
// 支持链式调用。
func (a *Application) Register(providers ...ServiceProvider) *Application {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.providers = append(a.providers, providers...)
	return a
}

// Container 返回底层容器。
func (a *Application) Container() Container {
	return a.container
}

// Boot 执行两阶段初始化：
//  1. 依次调用所有 Provider 的 Register()
//  2. 依次调用所有 Provider 的 Boot()
//
// Boot 是幂等的，重复调用不会产生副作用。
func (a *Application) Boot() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.booted {
		return nil
	}

	// Phase 1: Register — 统一绑定依赖。
	for _, p := range a.providers {
		if err := p.Register(a.container); err != nil {
			return fmt.Errorf("ioc: provider Register: %w", err)
		}
	}

	// Phase 2: Boot — 统一初始化依赖。
	for _, p := range a.providers {
		// DeferrableProvider 标记为延迟的，跳过 Boot 阶段。
		if dp, ok := p.(DeferrableProvider); ok && dp.Deferred() {
			continue
		}
		if err := p.Boot(a.container); err != nil {
			return fmt.Errorf("ioc: provider Boot: %w", err)
		}
	}

	a.booted = true
	return nil
}

// Shutdown 优雅关闭所有实现了 Closeable 接口的单例。
// 按创建的逆序执行 Close，确保依赖关系正确释放。
// 所有错误通过 errors.Join 聚合返回。
//
// 若底层 Container 未实现 containerInternals 接口（自定义实现），
// 则 Shutdown 为 no-op 并返回 nil。
func (a *Application) Shutdown(ctx context.Context) error {
	ci, ok := a.container.(containerInternals)
	if !ok {
		return nil
	}

	order := ci.Order()
	singletons := ci.Singletons()

	var errs []error
	// 反序遍历
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		val, exists := singletons[name]
		if !exists {
			continue
		}
		if closeable, ok := val.(Closeable); ok {
			if err := closeable.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("ioc: close %q: %w", name, err))
			}
		}
	}

	return errors.Join(errs...)
}

// HealthCheck 对所有实现了 HealthChecker 接口的单例执行健康检查。
// 返回 name → error 映射，error 为 nil 表示健康。
//
// 若底层 Container 未实现 containerInternals 接口，返回 nil。
func (a *Application) HealthCheck(ctx context.Context) map[string]error {
	ci, ok := a.container.(containerInternals)
	if !ok {
		return nil
	}

	singletons := ci.Singletons()
	result := make(map[string]error)

	for name, val := range singletons {
		if checker, ok := val.(HealthChecker); ok {
			result[name] = checker.Health(ctx)
		}
	}

	return result
}
