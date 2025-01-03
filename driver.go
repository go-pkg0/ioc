package ioc

import (
	"fmt"
	"sync"
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
//
// 示例:
//
//	mgr := ioc.NewDriverManager[cache.Store]("redis")
//	mgr.Register("redis", func() (cache.Store, error) {
//	    return redis.NewStore(cfg)
//	})
//	mgr.Register("memory", func() (cache.Store, error) {
//	    return memory.NewStore()
//	})
//	store, err := mgr.Default()  // 返回 redis store
type DriverManager[T Driver] interface {
	// Register 注册驱动工厂。
	Register(name string, factory func() (T, error))

	// Driver 按名称获取驱动实例（延迟创建 + 缓存）。
	Driver(name string) (T, error)

	// Default 获取默认驱动实例。
	Default() (T, error)

	// SetDefault 设置默认驱动名称。
	SetDefault(name string)

	// Extend 注册自定义驱动（允许覆盖已有驱动）。
	Extend(name string, factory func() (T, error))

	// Drivers 返回所有已注册驱动名称。
	Drivers() []string
}

// driverEntry 使用 sync.Once 保证驱动工厂仅执行一次。
type driverEntry[T any] struct {
	once sync.Once
	val  T
	err  error
}

// driverManager 是 DriverManager 的默认实现。
type driverManager[T Driver] struct {
	mu          sync.RWMutex
	defaultName string
	factories   map[string]func() (T, error)
	entries     map[string]*driverEntry[T]
}

// NewDriverManager 创建多驱动管理器实例。
// defaultName 指定默认驱动名称。
func NewDriverManager[T Driver](defaultName string) DriverManager[T] {
	return &driverManager[T]{
		defaultName: defaultName,
		factories:   make(map[string]func() (T, error)),
		entries:     make(map[string]*driverEntry[T]),
	}
}

func (m *driverManager[T]) Register(name string, factory func() (T, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[name] = factory
	// 清除旧 entry（支持重新注册）。
	delete(m.entries, name)
}

func (m *driverManager[T]) Driver(name string) (T, error) {
	// 快路径：读已完成的缓存。
	m.mu.RLock()
	if entry, ok := m.entries[name]; ok {
		m.mu.RUnlock()
		// entry.once 已完成，直接读结果。
		entry.once.Do(func() {}) // no-op，确保 happens-before
		if entry.err != nil {
			var zero T
			return zero, entry.err
		}
		return entry.val, nil
	}
	_, hasFactory := m.factories[name]
	m.mu.RUnlock()

	if !hasFactory {
		var zero T
		return zero, fmt.Errorf("%w: %q", ErrDriverNotFound, name)
	}

	// 获取或创建 entry。
	m.mu.Lock()
	entry, ok := m.entries[name]
	if !ok {
		entry = &driverEntry[T]{}
		m.entries[name] = entry
	}
	factory := m.factories[name]
	m.mu.Unlock()

	// sync.Once 保证工厂仅执行一次，且在锁外执行（避免死锁）。
	entry.once.Do(func() {
		entry.val, entry.err = factory()
	})

	if entry.err != nil {
		// 工厂出错：清除 entry，允许重试。
		m.mu.Lock()
		if m.entries[name] == entry {
			delete(m.entries, name)
		}
		m.mu.Unlock()
		var zero T
		return zero, fmt.Errorf("ioc: driver %q creation failed: %w", name, entry.err)
	}

	return entry.val, nil
}

func (m *driverManager[T]) Default() (T, error) {
	m.mu.RLock()
	name := m.defaultName
	m.mu.RUnlock()
	return m.Driver(name)
}

func (m *driverManager[T]) SetDefault(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultName = name
}

func (m *driverManager[T]) Extend(name string, factory func() (T, error)) {
	m.Register(name, factory)
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
