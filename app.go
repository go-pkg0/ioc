package ioc

import (
	"context"
	"fmt"
	"sync"
)

// appState Application 生命周期状态。
type appState int

const (
	appCreated      appState = iota // 初始状态
	appBooting                      // Boot 执行中
	appBooted                       // Boot 完成
	appShuttingDown                 // Shutdown 执行中
	appShutdown                     // Shutdown 完成
	appFailed                       // Boot 失败
)

func (s appState) String() string {
	switch s {
	case appCreated:
		return "created"
	case appBooting:
		return "booting"
	case appBooted:
		return "booted"
	case appShuttingDown:
		return "shutting_down"
	case appShutdown:
		return "shutdown"
	case appFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Application 编排 ServiceProvider 的生命周期。
//
// 不包含 HTTP 服务器、信号处理、配置加载等框架层逻辑，
// 这些由具体框架（如 q-gin）在上层实现。
//
// 生命周期状态机：created → booting → booted → shutting_down → shutdown
// Boot 失败时进入 failed 状态，仍可调用 Shutdown 清理资源。
type Application struct {
	mu        sync.Mutex
	container Container
	providers []ServiceProvider
	state     appState
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
// 在非 created 状态下调用会 panic（编程错误）。
// 支持链式调用。
func (a *Application) Register(providers ...ServiceProvider) *Application {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != appCreated {
		panic(fmt.Sprintf("ioc: cannot Register in state %q, must be %q", a.state, appCreated))
	}
	a.providers = append(a.providers, providers...)
	return a
}

// Container 返回底层容器。
func (a *Application) Container() Container {
	return a.container
}

// Booted 返回是否已成功 Boot。
func (a *Application) Booted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state == appBooted
}

// Boot 执行两阶段初始化：
//  1. 依次调用所有 Provider 的 Register()
//  2. 依次调用所有 Provider 的 Boot(ctx)
//
// Boot 从 booted 状态调用是幂等的（返回 nil）。
// Boot 失败后状态变为 failed，不可重试，但可调用 Shutdown 清理。
// ctx 用于控制 Boot 阶段的超时和取消。
func (a *Application) Boot(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch a.state {
	case appCreated:
		// 正常流程
	case appBooted:
		return nil // 幂等
	default:
		return fmt.Errorf("ioc: cannot Boot in state %q", a.state)
	}

	a.state = appBooting

	// Phase 1: Register — 统一绑定依赖。
	for _, p := range a.providers {
		if err := p.Register(a.container); err != nil {
			a.state = appFailed
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
			a.state = appFailed
			return fmt.Errorf("ioc: provider Boot: %w", err)
		}
	}

	a.state = appBooted
	return nil
}

// Shutdown 优雅关闭：委托给 Container.Close。
// 幂等：从 shutdown 状态调用返回 nil。
// 允许从 created/booted/failed 状态调用。
func (a *Application) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch a.state {
	case appShutdown:
		return nil // 幂等
	case appShuttingDown:
		return fmt.Errorf("ioc: shutdown already in progress")
	case appBooting:
		return fmt.Errorf("ioc: cannot shutdown during boot")
	}

	a.state = appShuttingDown
	err := a.container.Close(ctx)
	a.state = appShutdown
	return err
}

// HealthCheck 健康检查：委托给 Container.HealthCheck。
// 仅在 booted 状态下执行检查，其他状态返回 nil。
func (a *Application) HealthCheck(ctx context.Context) map[string]error {
	a.mu.Lock()
	state := a.state
	a.mu.Unlock()

	if state != appBooted {
		return nil
	}
	return a.container.HealthCheck(ctx)
}
