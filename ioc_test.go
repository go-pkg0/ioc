package ioc

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBind(t *testing.T) {
	c := New()

	callCount := 0
	c.Bind("svc", func(_ Container) (any, error) {
		callCount++
		return "instance", nil
	})

	// 每次 Make 都应创建新实例（调用工厂）
	v1, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}

	if v1 != "instance" || v2 != "instance" {
		t.Fatalf("got %v, %v", v1, v2)
	}
	if callCount != 2 {
		t.Fatalf("factory should be called twice for Bind, got %d", callCount)
	}
}

func TestSingleton(t *testing.T) {
	c := New()

	callCount := 0
	c.Singleton("svc", func(_ Container) (any, error) {
		callCount++
		return &struct{ Name string }{"singleton"}, nil
	})

	v1, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}

	// 单例应返回同一实例
	if v1 != v2 {
		t.Fatal("singleton should return same instance")
	}
	if callCount != 1 {
		t.Fatalf("factory should be called once for Singleton, got %d", callCount)
	}
}

func TestInstance(t *testing.T) {
	c := New()
	obj := &struct{ ID int }{42}
	c.Instance("svc", obj)

	v, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != obj {
		t.Fatal("Instance should return exact same pointer")
	}
}

func TestMustMakePanics(t *testing.T) {
	c := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustMake should panic on unregistered binding")
		}
	}()
	c.MustMake("nonexistent")
}

func TestHas(t *testing.T) {
	c := New()
	if c.Has("svc") {
		t.Fatal("should not have unregistered binding")
	}
	c.Bind("svc", func(_ Container) (any, error) { return nil, nil })
	if !c.Has("svc") {
		t.Fatal("should have registered binding")
	}
}

func TestRemove(t *testing.T) {
	c := New()
	c.Singleton("svc", func(_ Container) (any, error) { return "val", nil })
	c.Make("svc") // 触发缓存

	c.Remove("svc")

	if c.Has("svc") {
		t.Fatal("removed binding should not exist")
	}
	_, err := c.Make("svc")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestBindings(t *testing.T) {
	c := New()
	c.Bind("a", func(_ Container) (any, error) { return nil, nil })
	c.Singleton("b", func(_ Container) (any, error) { return nil, nil })
	c.Instance("c", "val")

	names := c.Bindings()
	if len(names) != 3 {
		t.Fatalf("expected 3 bindings, got %d", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, expected := range []string{"a", "b", "c"} {
		if !nameSet[expected] {
			t.Fatalf("missing binding %q", expected)
		}
	}
}

func TestFlush(t *testing.T) {
	c := New()
	c.Singleton("svc", func(_ Container) (any, error) { return "val", nil })
	c.Make("svc")

	c.Flush()

	if c.Has("svc") {
		t.Fatal("Flush should clear all bindings")
	}
	if len(c.Bindings()) != 0 {
		t.Fatal("Flush should clear all bindings")
	}
}

func TestMakeNotBound(t *testing.T) {
	c := New()
	_, err := c.Make("nonexistent")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestMakeFactoryError(t *testing.T) {
	c := New()
	expectedErr := errors.New("factory error")
	c.Singleton("svc", func(_ Container) (any, error) {
		return nil, expectedErr
	})

	_, err := c.Make("svc")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected factory error, got %v", err)
	}

	// 工厂出错后，单例不应被缓存，再次调用应重新执行工厂
	_, err = c.Make("svc")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected factory error on retry, got %v", err)
	}
}

func TestSingletonConcurrency(t *testing.T) {
	c := New()

	var count atomic.Int32
	c.Singleton("svc", func(_ Container) (any, error) {
		count.Add(1)
		return "value", nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.Make("svc")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if v != "value" {
				t.Errorf("unexpected value: %v", v)
			}
		}()
	}
	wg.Wait()

	// sync.Once 保证单例工厂恰好执行一次。
	if c := count.Load(); c != 1 {
		t.Fatalf("singleton factory called %d times, expected exactly 1", c)
	}
}

func TestDecorate(t *testing.T) {
	c := New()
	c.Singleton("svc", func(_ Container) (any, error) {
		return "base", nil
	})

	// 添加两个装饰器，验证管道顺序
	c.Decorate("svc", func(instance any, _ Container) (any, error) {
		return instance.(string) + "+d1", nil
	})
	c.Decorate("svc", func(instance any, _ Container) (any, error) {
		return instance.(string) + "+d2", nil
	})

	v, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "base+d1+d2" {
		t.Fatalf("expected 'base+d1+d2', got %q", v)
	}
}

func TestUseMiddleware(t *testing.T) {
	c := New()
	c.Bind("svc", func(_ Container) (any, error) {
		return "value", nil
	})

	var log []string
	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func() (any, error) {
			log = append(log, "mw1:before:"+abstract)
			v, err := next()
			log = append(log, "mw1:after:"+abstract)
			return v, err
		}
	})
	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func() (any, error) {
			log = append(log, "mw2:before:"+abstract)
			v, err := next()
			log = append(log, "mw2:after:"+abstract)
			return v, err
		}
	})

	v, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "value" {
		t.Fatalf("expected 'value', got %v", v)
	}

	// 洋葱模型：mw1 在外层
	expected := []string{
		"mw1:before:svc",
		"mw2:before:svc",
		"mw2:after:svc",
		"mw1:after:svc",
	}
	if len(log) != len(expected) {
		t.Fatalf("expected %d log entries, got %d: %v", len(expected), len(log), log)
	}
	for i, e := range expected {
		if log[i] != e {
			t.Fatalf("log[%d] = %q, want %q", i, log[i], e)
		}
	}
}

func TestSingletonCacheSkipsMiddlewareAndDecorator(t *testing.T) {
	c := New()

	var factoryCount, mwCount, decCount int
	c.Singleton("svc", func(_ Container) (any, error) {
		factoryCount++
		return "value", nil
	})
	c.Decorate("svc", func(instance any, _ Container) (any, error) {
		decCount++
		return instance, nil
	})
	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func() (any, error) {
			mwCount++
			return next()
		}
	})

	// 首次触发工厂 + 装饰器 + 中间件
	c.Make("svc")
	if factoryCount != 1 || decCount != 1 || mwCount != 1 {
		t.Fatalf("first Make: factory=%d, dec=%d, mw=%d", factoryCount, decCount, mwCount)
	}

	// 第二次走缓存，不触发任何链
	c.Make("svc")
	if factoryCount != 1 || decCount != 1 || mwCount != 1 {
		t.Fatalf("second Make should hit cache: factory=%d, dec=%d, mw=%d", factoryCount, decCount, mwCount)
	}
}

func TestNestedMake(t *testing.T) {
	c := New()
	c.Singleton("dep", func(_ Container) (any, error) {
		return "dependency", nil
	})
	c.Singleton("svc", func(c Container) (any, error) {
		dep, err := c.Make("dep")
		if err != nil {
			return nil, err
		}
		return "service+" + dep.(string), nil
	})

	v, err := c.Make("svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "service+dependency" {
		t.Fatalf("expected 'service+dependency', got %q", v)
	}
}
