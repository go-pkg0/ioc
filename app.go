package ioc

import (
	"context"
	"fmt"
	"sync"
)

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
//  2. 依次调用所有 Provider 的 Boot(ctx)
//
// Boot 是幂等的，重复调用不会产生副作用。
// ctx 用于控制 Boot 阶段的超时和取消。
func (a *Application) Boot(ctx context.Context) error {
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
		if err := p.Boot(ctx, a.container); err != nil {
			return fmt.Errorf("ioc: provider Boot: %w", err)
		}
	}

	a.booted = true
	return nil
}

// Shutdown 优雅关闭：委托给 Container.Close。
// 按创建的逆序关闭所有实现了 Closeable 接口的单例。
func (a *Application) Shutdown(ctx context.Context) error {
	return a.container.Close(ctx)
}

// HealthCheck 健康检查：委托给 Container.HealthCheck。
// 返回 name → error 映射，error 为 nil 表示健康。
func (a *Application) HealthCheck(ctx context.Context) map[string]error {
	return a.container.HealthCheck(ctx)
}
