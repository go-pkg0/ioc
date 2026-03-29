package ioc

import (
	"context"
	"errors"
	"testing"
)

func TestMakeTyped(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.Instance("name", "hello")

	v, err := MakeTyped[string](ctx, c, "name")
	if err != nil {
		t.Fatal(err)
	}
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestMakeTypedWrongType(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.Instance("num", 42)

	_, err := MakeTyped[string](ctx, c, "num")
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
	_, err := MakeTyped[string](ctx, c, "nonexistent")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestMustMakeTyped(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.Instance("name", "hello")

	v := MustMakeTyped[string](ctx, c, "name")
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestMustMakeTypedPanics(t *testing.T) {
	c := New()
	ctx := context.Background()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustMakeTyped should panic on missing binding")
		}
	}()
	MustMakeTyped[string](ctx, c, "nonexistent")
}

type myStruct struct{ Val int }

func TestMakeTypedStruct(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.Singleton("obj", func(_ context.Context, _ Container) (any, error) {
		return &myStruct{Val: 99}, nil
	})

	v, err := MakeTyped[*myStruct](ctx, c, "obj")
	if err != nil {
		t.Fatal(err)
	}
	if v.Val != 99 {
		t.Fatalf("expected 99, got %d", v.Val)
	}
}

func TestMakeTypedInterface(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.Instance("driver", &testDriver{name: "test"})

	v, err := MakeTyped[Driver](ctx, c, "driver")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name() != "test" {
		t.Fatalf("expected 'test', got %q", v.Name())
	}
}

func TestMakeTypedViaAlias(t *testing.T) {
	c := New()
	ctx := context.Background()
	c.Instance("database", "mysql")
	c.Alias("db", "database")

	v, err := MakeTyped[string](ctx, c, "db")
	if err != nil {
		t.Fatal(err)
	}
	if v != "mysql" {
		t.Fatalf("expected 'mysql', got %q", v)
	}
}
