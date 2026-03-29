package ioc

import (
	"context"
	"errors"
	"testing"
)

// --- Make / MustMake ---

func TestMake(t *testing.T) {
	c := New()
	ctx := context.Background()
	Instance(c, "name", "hello")

	v, err := Make[string](ctx, c, "name")
	if err != nil {
		t.Fatal(err)
	}
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestMakeWrongType(t *testing.T) {
	c := New()
	ctx := context.Background()
	Instance(c, "num", 42)

	_, err := Make[string](ctx, c, "num")
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("expected ErrTypeMismatch, got %v", err)
	}
}

func TestMakeTypedNotBound(t *testing.T) {
	c := New()
	ctx := context.Background()
	_, err := Make[string](ctx, c, "nonexistent")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestMustMakeSuccess(t *testing.T) {
	c := New()
	ctx := context.Background()
	Instance(c, "name", "hello")

	v := MustMake[string](ctx, c, "name")
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestMustMakePanicsOnMissing(t *testing.T) {
	c := New()
	ctx := context.Background()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustMake should panic on missing binding")
		}
	}()
	MustMake[string](ctx, c, "nonexistent")
}

// --- Singleton[T] / Bind[T] / Instance[T] ---

func TestSingletonTyped(t *testing.T) {
	c := New()
	ctx := context.Background()

	callCount := 0
	Singleton(c, "svc", func(_ context.Context, _ Container) (string, error) {
		callCount++
		return "singleton-value", nil
	})

	v1, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := Make[string](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v1 != v2 || v1 != "singleton-value" {
		t.Fatalf("expected 'singleton-value', got %q / %q", v1, v2)
	}
	if callCount != 1 {
		t.Fatalf("singleton factory should be called once, got %d", callCount)
	}
}

func TestBindTyped(t *testing.T) {
	c := New()
	ctx := context.Background()

	callCount := 0
	Bind(c, "svc", func(_ context.Context, _ Container) (int, error) {
		callCount++
		return callCount, nil
	})

	v1, err := Make[int](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := Make[int](ctx, c, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 {
		t.Fatal("Bind should create new instance each time")
	}
	if callCount != 2 {
		t.Fatalf("factory should be called twice, got %d", callCount)
	}
}

func TestInstanceTyped(t *testing.T) {
	c := New()
	ctx := context.Background()
	Instance(c, "port", 8080)

	v, err := Make[int](ctx, c, "port")
	if err != nil {
		t.Fatal(err)
	}
	if v != 8080 {
		t.Fatalf("expected 8080, got %d", v)
	}
}

// --- Decorate[T] ---

func TestDecorateTyped(t *testing.T) {
	c := New()
	ctx := context.Background()

	Singleton(c, "greeting", func(_ context.Context, _ Container) (string, error) {
		return "hello", nil
	})

	Decorate(c, "greeting", func(_ context.Context, val string, _ Container) (string, error) {
		return val + " world", nil
	})
	Decorate(c, "greeting", func(_ context.Context, val string, _ Container) (string, error) {
		return val + "!", nil
	})

	v, err := Make[string](ctx, c, "greeting")
	if err != nil {
		t.Fatal(err)
	}
	if v != "hello world!" {
		t.Fatalf("expected 'hello world!', got %q", v)
	}
}

func TestDecorateTypedMismatch(t *testing.T) {
	c := New()
	ctx := context.Background()

	// 使用 Singleton（非 Instance），确保装饰器被执行
	Singleton(c, "num", func(_ context.Context, _ Container) (int, error) {
		return 42, nil
	})

	// 注册一个期望 string 的装饰器，但工厂返回 int
	Decorate(c, "num", func(_ context.Context, val string, _ Container) (string, error) {
		return val + "!", nil
	})

	_, err := c.Make(ctx, "num")
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("expected ErrTypeMismatch, got %v", err)
	}
}

// --- 结构体和接口 ---

type myStruct struct{ Val int }

func TestMakeStruct(t *testing.T) {
	c := New()
	ctx := context.Background()
	Singleton(c, "obj", func(_ context.Context, _ Container) (*myStruct, error) {
		return &myStruct{Val: 99}, nil
	})

	v, err := Make[*myStruct](ctx, c, "obj")
	if err != nil {
		t.Fatal(err)
	}
	if v.Val != 99 {
		t.Fatalf("expected 99, got %d", v.Val)
	}
}

func TestMakeInterface(t *testing.T) {
	c := New()
	ctx := context.Background()
	Instance(c, "driver", &testDriver{name: "test"})

	v, err := Make[Driver](ctx, c, "driver")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name() != "test" {
		t.Fatalf("expected 'test', got %q", v.Name())
	}
}

// --- 泛型 + 别名 ---

func TestMakeViaAlias(t *testing.T) {
	c := New()
	ctx := context.Background()
	Instance(c, "database", "mysql")
	c.Alias("db", "database")

	v, err := Make[string](ctx, c, "db")
	if err != nil {
		t.Fatal(err)
	}
	if v != "mysql" {
		t.Fatalf("expected 'mysql', got %q", v)
	}
}

// --- 泛型嵌套 Make ---

func TestSingletonTypedNested(t *testing.T) {
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
