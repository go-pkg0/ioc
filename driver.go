package ioc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// Driver 驱动接口 — 所有驱动实现的最小公约。
type Driver interface {
	// Name 返回驱动名称（如 "mysql", "redis", "file"）。
	Name() string
}

// DriverManager 多驱动管理器泛型接口。
//
// 为 db、cache、log 等需要多驱动切换的模块提供统一管理模式。
// T 为具体驱动接口（如 db.Connection, cache.Store, log.Writer），
// 必须实现 Driver 接口。
//
// 驱动实例延迟创建：仅在首次调用 Driver(name) 时执行工厂函数，
// 创建后的实例会被缓存，后续调用直接返回。
// 若工厂返回错误，entry 会被清除以允许后续调用者重试。
//
// 示例:
//
//	mgr := ioc.NewDriverManager[cache.Store]("redis")
//	mgr.Register("redis", func(ctx context.Context) (cache.Store, error) {
//	    return redis.NewStore(cfg)
//	})
//	mgr.Register("memory", func(ctx context.Context) (cache.Store, error) {
//	    return memory.NewStore()
//	})
//	store, err := mgr.Default(ctx)  // 返回 redis store
type DriverManager[T Driver] interface {
	// Register 注册驱动工厂。
	// 管理器关闭后调用 panic（编程错误）。
	Register(name string, factory func(ctx context.Context) (T, error))

	// Driver 按名称获取驱动实例（延迟创建 + 缓存）。
	// ctx 传递给工厂函数，用于超时控制和链路追踪。
	// 管理器关闭后返回 ErrDriverManagerClosed。
	Driver(ctx context.Context, name string) (T, error)

	// Default 获取默认驱动实例。
	Default(ctx context.Context) (T, error)

	// SetDefault 设置默认驱动名称。
	// 管理器关闭后调用 panic（编程错误）。
	SetDefault(name string)

	// Extend 装饰已注册的驱动。
	// extender 接收原始驱动实例并返回增强后的实例。
	// 原始驱动的工厂必须已通过 Register 注册，否则 panic。
	// 调用后会清除该驱动的缓存，下次获取时重新创建。
	// 管理器关闭后调用 panic（编程错误）。
	Extend(name string, extender func(original T) (T, error))

	// Drivers 返回所有已注册驱动名称。
	Drivers() []string

	// Close 优雅关闭所有已创建且实现了 Closeable 接口的驱动实例。
	// 幂等：重复调用返回 nil。关闭后 Driver 调用返回 ErrDriverManagerClosed。
	Close(ctx context.Context) error
}

// driverEntry 使用 sync.Once 保证驱动工厂仅执行一次。
// factory 必须在创建 entry 时赋值，确保任何 goroutine 赢得 Once 竞争都能执行真实工厂。
// 若工厂失败，entry 会从 entries 中清除以允许后续重试。
type driverEntry[T any] struct {
	once    sync.Once
	factory func(ctx context.Context) (T, error) // 创建时赋值，不可变
	val     T
	err     error
}

// resolve 通过 sync.Once 保证工厂仅执行一次。
// 任何 goroutine 调用 resolve 都安全：仅第一个执行工厂，其余等待。
func (e *driverEntry[T]) resolve(ctx context.Context) {
	e.once.Do(func() {
		e.val, e.err = e.factory(ctx)
	})
}

// driverManager 是 DriverManager 的默认实现。
type driverManager[T Driver] struct {
	closed      atomic.Bool
	mu          sync.RWMutex
	defaultName string
	factories   map[string]func(ctx context.Context) (T, error)
	entries     map[string]*driverEntry[T]
}

// NewDriverManager 创建多驱动管理器实例。
// defaultName 指定默认驱动名称。
func NewDriverManager[T Driver](defaultName string) DriverManager[T] {
	return &driverManager[T]{
		defaultName: defaultName,
		factories:   make(map[string]func(ctx context.Context) (T, error)),
		entries:     make(map[string]*driverEntry[T]),
	}
}

func (m *driverManager[T]) Register(name string, factory func(ctx context.Context) (T, error)) {
	if m.closed.Load() {
		panic("ioc: Register on closed DriverManager")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[name] = factory
	// 清除旧 entry（支持重新注册）。
	delete(m.entries, name)
}

func (m *driverManager[T]) Driver(ctx context.Context, name string) (T, error) {
	if m.closed.Load() {
		var zero T
		return zero, ErrDriverManagerClosed
	}

	// 快路径：entry 已存在（可能已完成或正在创建中）。
	m.mu.RLock()
	entry, cached := m.entries[name]
	_, registered := m.factories[name]
	m.mu.RUnlock()

	if !cached && !registered {
		var zero T
		return zero, fmt.Errorf("%w: %q", ErrDriverNotFound, name)
	}

	if cached {
		// entry 存在 — 等待创建完成（已完成时为无开销的原子检查）。
		entry.resolve(ctx)
		if entry.err != nil {
			m.clearFailedEntry(name, entry)
			var zero T
			return zero, entry.err
		}
		return entry.val, nil
	}

	// 慢路径：创建 entry。
	m.mu.Lock()
	// 关闭保护：在写锁内二次检查。
	if m.closed.Load() {
		m.mu.Unlock()
		var zero T
		return zero, ErrDriverManagerClosed
	}
	// 双重检查：可能在等待锁期间已被其他 goroutine 创建。
	entry, cached = m.entries[name]
	if cached {
		m.mu.Unlock()
		entry.resolve(ctx)
		if entry.err != nil {
			m.clearFailedEntry(name, entry)
			var zero T
			return zero, entry.err
		}
		return entry.val, nil
	}
	factory := m.factories[name]
	entry = &driverEntry[T]{factory: factory}
	m.entries[name] = entry
	m.mu.Unlock()

	// resolve 通过 sync.Once 保证工厂仅执行一次，且在锁外执行（避免死锁）。
	entry.resolve(ctx)

	// 工厂失败：清除 entry，允许后续调用者用新 context 重试。
	if entry.err != nil {
		m.clearFailedEntry(name, entry)
		var zero T
		return zero, entry.err
	}

	return entry.val, nil
}

func (m *driverManager[T]) Default(ctx context.Context) (T, error) {
	m.mu.RLock()
	name := m.defaultName
	m.mu.RUnlock()
	return m.Driver(ctx, name)
}

func (m *driverManager[T]) SetDefault(name string) {
	if m.closed.Load() {
		panic("ioc: SetDefault on closed DriverManager")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultName = name
}

func (m *driverManager[T]) Extend(name string, extender func(original T) (T, error)) {
	if m.closed.Load() {
		panic("ioc: Extend on closed DriverManager")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	originalFactory, exists := m.factories[name]
	if !exists {
		panic(fmt.Sprintf("ioc: cannot extend unregistered driver %q, call Register first", name))
	}

	m.factories[name] = func(ctx context.Context) (T, error) {
		base, err := originalFactory(ctx)
		if err != nil {
			var zero T
			return zero, err
		}
		return extender(base)
	}
	// 清除缓存，下次获取时重新创建。
	delete(m.entries, name)
}

func (m *driverManager[T]) Drivers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.factories))
	for name := range m.factories {
		names = append(names, name)
	}
	return names
}

// Close 优雅关闭所有已创建且实现了 Closeable 接口的驱动实例。
// 幂等：重复调用返回 nil。关闭后 Driver 调用返回 ErrDriverManagerClosed。
func (m *driverManager[T]) Close(ctx context.Context) error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil // 已关闭，幂等
	}

	m.mu.RLock()
	entries := make(map[string]*driverEntry[T], len(m.entries))
	for k, v := range m.entries {
		entries[k] = v
	}
	m.mu.RUnlock()

	var errs []error
	for name, entry := range entries {
		// 等待任何进行中的创建完成（entry.factory 已赋值，安全）。
		entry.resolve(ctx)
		if entry.err != nil {
			continue
		}
		if closeable, ok := any(entry.val).(Closeable); ok {
			if err := closeable.Close(ctx); err != nil {
				errs = append(errs, fmt.Errorf("ioc: close driver %q: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// clearFailedEntry 清除失败的 entry，允许后续调用者重试。
// 使用指针比较确保只清除当前 entry，不影响并发重新注册创建的新 entry。
func (m *driverManager[T]) clearFailedEntry(name string, entry *driverEntry[T]) {
	m.mu.Lock()
	if cur, ok := m.entries[name]; ok && cur == entry {
		delete(m.entries, name)
	}
	m.mu.Unlock()
}
