package ioc

import (
	"fmt"
	"sync"
)

// 编译期断言：*container 实现 Container 接口。
var _ Container = (*container)(nil)

// singletonEntry 使用 sync.Once 保证单例工厂仅执行一次。
type singletonEntry struct {
	once sync.Once
	val  any
	err  error
}

// container 是 Container 的默认实现。
type container struct {
	mu          sync.RWMutex
	bindings    map[string]binding
	singletons  map[string]any              // 已就绪的单例缓存
	pending     map[string]*singletonEntry  // 正在创建的单例（per-key Once）
	order       []string                    // 单例创建顺序（Shutdown 反序关闭用）
	decorators  map[string][]Decorator      // 服务级装饰器
	middlewares []Middleware                 // 全局中间件
}

// New 创建一个空的 Container。
func New() Container {
	return &container{
		bindings:   make(map[string]binding),
		singletons: make(map[string]any),
		pending:    make(map[string]*singletonEntry),
		decorators: make(map[string][]Decorator),
	}
}

// Bind 注册瞬时工厂。
func (c *container) Bind(abstract string, factory Factory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings[abstract] = binding{factory: factory, singleton: false}
}

// Singleton 注册单例工厂。
func (c *container) Singleton(abstract string, factory Factory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings[abstract] = binding{factory: factory, singleton: true}
	// 清除旧的 pending entry（重新注册场景）。
	delete(c.pending, abstract)
}

// Instance 直接注册已构建的实例为单例。
func (c *container) Instance(abstract string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.singletons[abstract] = value
	c.bindings[abstract] = binding{singleton: true}
	c.order = append(c.order, abstract)
}

// Make 按名称解析服务实例。
func (c *container) Make(abstract string) (any, error) {
	// 快路径：单例缓存命中，直接返回（零分配，不经过中间件/装饰器）。
	c.mu.RLock()
	if val, ok := c.singletons[abstract]; ok {
		c.mu.RUnlock()
		return val, nil
	}
	b, hasBind := c.bindings[abstract]
	// 安全拷贝 slice，防止并发 append 导致的竞态。
	decs := copySlice(c.decorators[abstract])
	mws := copySlice(c.middlewares)
	c.mu.RUnlock()

	if !hasBind {
		return nil, fmt.Errorf("%w: %q", ErrNotBound, abstract)
	}

	if b.factory == nil {
		return nil, fmt.Errorf("%w: %q", ErrNoFactory, abstract)
	}

	// 构建解析函数：工厂 → 装饰器链。
	resolve := func() (any, error) {
		val, err := b.factory(c)
		if err != nil {
			return nil, err
		}
		// 按注册顺序执行装饰器（管道模型）。
		for _, dec := range decs {
			val, err = dec(val, c)
			if err != nil {
				return nil, err
			}
		}
		return val, nil
	}

	// 全局中间件包装（反序，使先注册的在最外层——洋葱模型）。
	for i := len(mws) - 1; i >= 0; i-- {
		resolve = mws[i](abstract, resolve)
	}

	// 瞬时绑定：每次创建新实例。
	if !b.singleton {
		return resolve()
	}

	// 单例路径：使用 per-key sync.Once 保证工厂仅执行一次。
	// 获取或创建 singletonEntry。
	c.mu.Lock()
	// 二次检查：可能在等待锁期间已被其他 goroutine 创建。
	if val, ok := c.singletons[abstract]; ok {
		c.mu.Unlock()
		return val, nil
	}
	entry, ok := c.pending[abstract]
	if !ok {
		entry = &singletonEntry{}
		c.pending[abstract] = entry
	}
	c.mu.Unlock()

	// sync.Once 保证 resolve 只执行一次，且在锁外执行（避免死锁）。
	entry.once.Do(func() {
		entry.val, entry.err = resolve()
	})

	if entry.err != nil {
		// 工厂出错：清除 pending entry，允许重试。
		c.mu.Lock()
		if c.pending[abstract] == entry {
			delete(c.pending, abstract)
		}
		c.mu.Unlock()
		return nil, entry.err
	}

	// 将结果提升到 singletons 缓存（快路径）。
	c.mu.Lock()
	if _, ok := c.singletons[abstract]; !ok {
		c.singletons[abstract] = entry.val
		c.order = append(c.order, abstract)
	}
	delete(c.pending, abstract)
	c.mu.Unlock()

	return entry.val, nil
}

// MustMake 解析服务实例，失败时 panic。
func (c *container) MustMake(abstract string) any {
	val, err := c.Make(abstract)
	if err != nil {
		panic(err)
	}
	return val
}

// Has 判断指定名称是否已注册。
func (c *container) Has(abstract string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.bindings[abstract]
	return ok
}

// Decorate 为指定服务添加装饰器。
func (c *container) Decorate(abstract string, decorator Decorator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decorators[abstract] = append(c.decorators[abstract], decorator)
}

// Use 注册全局中间件。
func (c *container) Use(middleware ...Middleware) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.middlewares = append(c.middlewares, middleware...)
}

// Remove 删除指定名称的绑定及其缓存。
func (c *container) Remove(abstract string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bindings, abstract)
	delete(c.singletons, abstract)
	delete(c.decorators, abstract)
	delete(c.pending, abstract)
	// 从 order 中移除
	c.removeFromOrder(abstract)
}

// removeFromOrder 从 order 切片中移除指定名称（必须持有写锁）。
func (c *container) removeFromOrder(abstract string) {
	for i := 0; i < len(c.order); i++ {
		if c.order[i] == abstract {
			c.order = append(c.order[:i], c.order[i+1:]...)
			i--
		}
	}
}

// Bindings 返回所有已注册名称的快照。
func (c *container) Bindings() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.bindings))
	for k := range c.bindings {
		names = append(names, k)
	}
	return names
}

// Flush 清空所有绑定、单例缓存和装饰器。
func (c *container) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings = make(map[string]binding)
	c.singletons = make(map[string]any)
	c.pending = make(map[string]*singletonEntry)
	c.decorators = make(map[string][]Decorator)
	c.middlewares = nil
	c.order = nil
}

// Order 返回单例创建顺序（用于 Application 反序关闭）。
func (c *container) Order() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.order))
	copy(out, c.order)
	return out
}

// Singletons 返回所有已缓存的单例快照（用于 Application 健康检查等）。
func (c *container) Singletons() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]any, len(c.singletons))
	for k, v := range c.singletons {
		out[k] = v
	}
	return out
}

// copySlice 安全拷贝切片元素，防止并发 append 修改 backing array。
func copySlice[T any](src []T) []T {
	if len(src) == 0 {
		return nil
	}
	dst := make([]T, len(src))
	copy(dst, src)
	return dst
}
