package ioc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// panicHealthService 测试 HealthCheck 的 panic 恢复。
type panicHealthService struct{}

func (s *panicHealthService) Health(_ context.Context) error {
	panic("health check exploded")
}

func TestBind(t *testing.T) {
	c := New()
	ctx := context.Background()

	callCount := 0
	Bind(c, "svc", func(_ context.Context, _ Container) (string, error) {
		callCount++
		return "instance", nil
	})

	v1, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := Make[string](ctx, c, "svc")
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
	ctx := context.Background()

	type svc struct{ Name string }
	callCount := 0
	Singleton(c, "svc", func(_ context.Context, _ Container) (*svc, error) {
		callCount++
		return &svc{Name: "singleton"}, nil
	})

	v1, err := Make[*svc](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := Make[*svc](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}

	if v1 != v2 {
		t.Fatal("singleton should return same instance")
	}
	if callCount != 1 {
		t.Fatalf("factory should be called once for Singleton, got %d", callCount)
	}
}

func TestInstance(t *testing.T) {
	c := New()
	ctx := context.Background()
	type obj struct{ ID int }
	o := &obj{42}
	Instance(c, "svc", o)

	v, err := Make[*obj](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != o {
		t.Fatal("Instance should return exact same pointer")
	}
}

func TestMustMakePanics(t *testing.T) {
	c := New()
	ctx := context.Background()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustMake should panic on unregistered binding")
		}
	}()
	c.MustMake(ctx, "nonexistent")
}

func TestHas(t *testing.T) {
	c := New()
	if c.Has("svc") {
		t.Fatal("should not have unregistered binding")
	}
	Bind(c, "svc", func(_ context.Context, _ Container) (string, error) { return "", nil })
	if !c.Has("svc") {
		t.Fatal("should have registered binding")
	}
}

func TestRemove(t *testing.T) {
	c := New()
	ctx := context.Background()
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) { return "val", nil })
	Make[string](ctx, c, "svc")

	c.Remove("svc")

	if c.Has("svc") {
		t.Fatal("removed binding should not exist")
	}
	_, err := c.Make(ctx, "svc")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestBindings(t *testing.T) {
	c := New()
	Bind(c, "a", func(_ context.Context, _ Container) (string, error) { return "", nil })
	Singleton(c, "b", func(_ context.Context, _ Container) (string, error) { return "", nil })
	Instance(c, "c", "val")

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
	ctx := context.Background()
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) { return "val", nil })
	Make[string](ctx, c, "svc")
	c.Alias("alias", "svc")

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
	ctx := context.Background()
	_, err := c.Make(ctx, "nonexistent")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestMakeFactoryError(t *testing.T) {
	c := New()
	ctx := context.Background()

	callCount := 0
	expectedErr := errors.New("factory error")
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		callCount++
		return "", expectedErr
	})

	_, err := Make[string](ctx, c, "svc")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected factory error, got %v", err)
	}

	// 工厂失败后允许重试：后续调用会重新执行工厂
	_, err = Make[string](ctx, c, "svc")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected factory error on retry, got %v", err)
	}
	if callCount != 2 {
		t.Fatalf("factory should be retried on failure, got %d calls", callCount)
	}
}

func TestMakeFactoryErrorRetryAfterRemove(t *testing.T) {
	c := New()
	ctx := context.Background()

	callCount := 0
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		callCount++
		return "", errors.New("transient error")
	})

	_, err := Make[string](ctx, c, "svc")
	if err == nil {
		t.Fatal("expected error")
	}

	c.Remove("svc")
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		callCount++
		return "success", nil
	})

	v, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatalf("expected success after re-register, got %v", err)
	}
	if v != "success" {
		t.Fatalf("expected 'success', got %v", v)
	}
}

func TestSingletonConcurrency(t *testing.T) {
	c := New()
	ctx := context.Background()

	var count atomic.Int32
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		count.Add(1)
		return "value", nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := Make[string](ctx, c, "svc")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if v != "value" {
				t.Errorf("unexpected value: %v", v)
			}
		}()
	}
	wg.Wait()

	if c := count.Load(); c != 1 {
		t.Fatalf("singleton factory called %d times, expected exactly 1", c)
	}
}

func TestDecorate(t *testing.T) {
	c := New()
	ctx := context.Background()
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		return "base", nil
	})

	Decorate(c, "svc", func(_ context.Context, val string, _ Container) (string, error) {
		return val + "+d1", nil
	})
	Decorate(c, "svc", func(_ context.Context, val string, _ Container) (string, error) {
		return val + "+d2", nil
	})

	v, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "base+d1+d2" {
		t.Fatalf("expected 'base+d1+d2', got %q", v)
	}
}

func TestUseMiddleware(t *testing.T) {
	c := New()
	ctx := context.Background()
	Bind(c, "svc", func(_ context.Context, _ Container) (string, error) {
		return "value", nil
	})

	var log []string
	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func(ctx context.Context) (any, error) {
			log = append(log, "mw1:before:"+abstract)
			v, err := next(ctx)
			log = append(log, "mw1:after:"+abstract)
			return v, err
		}
	})
	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func(ctx context.Context) (any, error) {
			log = append(log, "mw2:before:"+abstract)
			v, err := next(ctx)
			log = append(log, "mw2:after:"+abstract)
			return v, err
		}
	})

	v, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "value" {
		t.Fatalf("expected 'value', got %v", v)
	}

	expected := []string{
		"mw1:before:svc", "mw2:before:svc",
		"mw2:after:svc", "mw1:after:svc",
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
	ctx := context.Background()

	var factoryCount, mwCount, decCount int
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		factoryCount++
		return "value", nil
	})
	Decorate(c, "svc", func(_ context.Context, val string, _ Container) (string, error) {
		decCount++
		return val, nil
	})
	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func(ctx context.Context) (any, error) {
			mwCount++
			return next(ctx)
		}
	})

	Make[string](ctx, c, "svc")
	if factoryCount != 1 || decCount != 1 || mwCount != 1 {
		t.Fatalf("first Make: factory=%d, dec=%d, mw=%d", factoryCount, decCount, mwCount)
	}

	Make[string](ctx, c, "svc")
	if factoryCount != 1 || decCount != 1 || mwCount != 1 {
		t.Fatalf("second Make should hit cache: factory=%d, dec=%d, mw=%d", factoryCount, decCount, mwCount)
	}
}

func TestNestedMake(t *testing.T) {
	c := New()
	ctx := context.Background()
	Singleton(c, "dep", func(_ context.Context, _ Container) (string, error) {
		return "dependency", nil
	})
	Singleton(c, "svc", func(ctx context.Context, c Container) (string, error) {
		dep, err := Make[string](ctx, c, "dep")
		if err != nil {
			return "", err
		}
		return "service+" + dep, nil
	})

	v, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "service+dependency" {
		t.Fatalf("expected 'service+dependency', got %q", v)
	}
}

func TestMiddlewareContextPropagation(t *testing.T) {
	c := New()
	type ctxKey string
	key := ctxKey("trace-id")

	Bind(c, "svc", func(ctx context.Context, _ Container) (string, error) {
		v, _ := ctx.Value(key).(string)
		return v, nil
	})

	c.Use(func(abstract string, next ResolveFunc) ResolveFunc {
		return func(ctx context.Context) (any, error) {
			ctx = context.WithValue(ctx, key, "abc-123")
			return next(ctx)
		}
	})

	v, err := Make[string](context.Background(), c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v != "abc-123" {
		t.Fatalf("expected 'abc-123', got %v", v)
	}
}

// --- P0: 循环依赖检测 ---

func TestCircularDependencySelf(t *testing.T) {
	c := New()
	Singleton(c, "A", func(ctx context.Context, c Container) (string, error) {
		return Make[string](ctx, c, "A")
	})

	_, err := c.Make(context.Background(), "A")
	if !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("expected ErrCircularDependency, got %v", err)
	}
}

func TestCircularDependencyChain(t *testing.T) {
	c := New()
	Singleton(c, "A", func(ctx context.Context, c Container) (string, error) {
		return Make[string](ctx, c, "B")
	})
	Singleton(c, "B", func(ctx context.Context, c Container) (string, error) {
		return Make[string](ctx, c, "C")
	})
	Singleton(c, "C", func(ctx context.Context, c Container) (string, error) {
		return Make[string](ctx, c, "A")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := c.Make(ctx, "A")
	if !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("expected ErrCircularDependency, got %v", err)
	}

	// 错误消息应包含完整链
	errMsg := err.Error()
	if errMsg == "" {
		t.Fatal("error message should not be empty")
	}
}

func TestCircularDependencyTransient(t *testing.T) {
	c := New()
	Bind(c, "A", func(ctx context.Context, c Container) (string, error) {
		return Make[string](ctx, c, "A")
	})

	_, err := c.Make(context.Background(), "A")
	if !errors.Is(err, ErrCircularDependency) {
		t.Fatalf("expected ErrCircularDependency for transient, got %v", err)
	}
}

// --- P0: Container closed 状态 ---

func TestContainerClosedBlocksMake(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "svc", "value")
	c.Close(ctx)

	_, err := c.Make(ctx, "svc")
	if !errors.Is(err, ErrContainerClosed) {
		t.Fatalf("expected ErrContainerClosed, got %v", err)
	}
}

func TestContainerCloseIdempotent(t *testing.T) {
	c := New()
	ctx := context.Background()

	svc := &closeableService{name: "svc"}
	Instance(c, "svc", svc)

	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 第二次关闭应为 no-op，不会重复调用 Close
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestContainerClose(t *testing.T) {
	c := New()
	ctx := context.Background()

	svc1 := &closeableService{name: "first"}
	svc2 := &closeableService{name: "second"}
	Instance(c, "first", svc1)
	Instance(c, "second", svc2)

	err := c.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !svc1.closed || !svc2.closed {
		t.Fatal("all closeable services should be closed")
	}
}

func TestContainerHealthCheck(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "healthy", &healthyService{})
	Instance(c, "unhealthy", &unhealthyService{})
	Instance(c, "plain", "not-a-health-checker")

	result := c.HealthCheck(ctx)

	if len(result) != 2 {
		t.Fatalf("expected 2 health checks, got %d", len(result))
	}
	if result["healthy"] != nil {
		t.Fatal("healthy service should return nil error")
	}
	if result["unhealthy"] == nil {
		t.Fatal("unhealthy service should return error")
	}
}

// --- P0: Instance/Singleton order 去重 ---

func TestInstanceReregistrationNoDuplicateOrder(t *testing.T) {
	c := New()
	ctx := context.Background()

	svc := &closeableService{name: "svc"}
	Instance(c, "svc", "old")
	Instance(c, "svc", svc) // 重复注册

	closeCount := 0
	svc.onClose = func() { closeCount++ }

	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if closeCount != 1 {
		t.Fatalf("Close should be called exactly once, got %d", closeCount)
	}
}

func TestSingletonReregistrationNoDuplicateOrder(t *testing.T) {
	c := New()
	ctx := context.Background()

	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		return "v1", nil
	})
	Make[string](ctx, c, "svc") // 触发缓存 + order 追加

	// 重新注册
	Singleton(c, "svc", func(_ context.Context, _ Container) (*closeableService, error) {
		return &closeableService{name: "v2"}, nil
	})
	Make[*closeableService](ctx, c, "svc") // 触发新工厂

	// Close 内部 order 不应有重复
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

// --- P2: Alias ---

func TestAlias(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "database", "mysql-conn")
	c.Alias("db", "database")

	v, err := Make[string](ctx, c, "db")
	if err != nil {
		t.Fatal(err)
	}
	if v != "mysql-conn" {
		t.Fatalf("expected 'mysql-conn', got %v", v)
	}
}

func TestAliasChain(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "database", "conn")
	c.Alias("db", "database")
	c.Alias("storage", "db")

	v, err := Make[string](ctx, c, "storage")
	if err != nil {
		t.Fatal(err)
	}
	if v != "conn" {
		t.Fatalf("expected 'conn', got %v", v)
	}
}

func TestHasResolveAlias(t *testing.T) {
	c := New()
	Instance(c, "database", "conn")
	c.Alias("db", "database")

	if !c.Has("db") {
		t.Fatal("Has should resolve alias")
	}
}

func TestFlushResetsClosedState(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "svc", "value")
	c.Close(ctx)

	// Close 后 Make 应返回 ErrContainerClosed
	_, err := c.Make(ctx, "svc")
	if !errors.Is(err, ErrContainerClosed) {
		t.Fatalf("expected ErrContainerClosed, got %v", err)
	}

	// Flush 重置关闭状态
	c.Flush()
	Instance(c, "svc", "new-value")

	v, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatalf("Flush should reset closed state, got %v", err)
	}
	if v != "new-value" {
		t.Fatalf("expected 'new-value', got %q", v)
	}
}

func TestCircularAliasPanics(t *testing.T) {
	c := New()

	c.Alias("a", "b")
	c.Alias("b", "a")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("circular alias should panic")
		}
	}()

	c.Has("a") // triggers resolveAliasLocked
}

func TestAliasChainTooDeepPanics(t *testing.T) {
	c := New()

	// 创建超过 10 层的别名链
	for i := 0; i < 12; i++ {
		c.Alias(fmt.Sprintf("a%d", i), fmt.Sprintf("a%d", i+1))
	}
	Instance(c, "a12", "value")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("alias chain too deep should panic")
		}
	}()

	c.Has("a0") // triggers resolveAliasLocked
}

func TestHealthCheckPanicRecovery(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "panicker", &panicHealthService{})
	Instance(c, "healthy", &healthyService{})

	result := c.HealthCheck(ctx)
	if len(result) != 2 {
		t.Fatalf("expected 2 health checks, got %d", len(result))
	}
	if result["healthy"] != nil {
		t.Fatal("healthy service should return nil error")
	}
	if result["panicker"] == nil {
		t.Fatal("panicking service should return error")
	}
}

func TestOperationsOnClosedContainerPanic(t *testing.T) {
	ops := map[string]func(Container){
		"Bind": func(c Container) {
			c.Bind("x", func(_ context.Context, _ Container) (any, error) { return nil, nil })
		},
		"Singleton": func(c Container) {
			c.Singleton("x", func(_ context.Context, _ Container) (any, error) { return nil, nil })
		},
		"Instance": func(c Container) {
			c.Instance("x", "val")
		},
		"Alias": func(c Container) {
			c.Alias("x", "y")
		},
		"Decorate": func(c Container) {
			c.Decorate("x", func(_ context.Context, v any, _ Container) (any, error) { return v, nil })
		},
		"Use": func(c Container) {
			c.Use(func(_ string, next ResolveFunc) ResolveFunc { return next })
		},
	}

	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Close(context.Background())

			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("%s on closed container should panic", name)
				}
			}()
			op(c)
		})
	}
}

func TestSingletonFactoryRetryOnTransientError(t *testing.T) {
	c := New()
	ctx := context.Background()

	callCount := 0
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		callCount++
		if callCount == 1 {
			return "", errors.New("transient error")
		}
		return "success", nil
	})

	// 首次调用失败
	_, err := Make[string](ctx, c, "svc")
	if err == nil {
		t.Fatal("expected error on first call")
	}

	// 重试应成功
	v, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatalf("expected success on retry, got %v", err)
	}
	if v != "success" {
		t.Fatalf("expected 'success', got %q", v)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 factory calls, got %d", callCount)
	}
}

func TestRemoveAlias(t *testing.T) {
	c := New()
	ctx := context.Background()

	Instance(c, "database", "conn")
	c.Alias("db", "database")

	c.Remove("database")

	if c.Has("db") {
		t.Fatal("alias should be removed when target is removed")
	}
	_, err := c.Make(ctx, "db")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound after remove, got %v", err)
	}
}
