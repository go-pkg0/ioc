package ioc

import (
	"errors"
	"testing"
)

func TestMakeTyped(t *testing.T) {
	c := New()
	c.Instance("name", "hello")

	v, err := MakeTyped[string](c, "name")
	if err != nil {
		t.Fatal(err)
	}
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestMakeTypedWrongType(t *testing.T) {
	c := New()
	c.Instance("num", 42)

	_, err := MakeTyped[string](c, "num")
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
}

func TestMakeTypedNotBound(t *testing.T) {
	c := New()
	_, err := MakeTyped[string](c, "nonexistent")
	if !errors.Is(err, ErrNotBound) {
		t.Fatalf("expected ErrNotBound, got %v", err)
	}
}

func TestMustMakeTyped(t *testing.T) {
	c := New()
	c.Instance("name", "hello")

	v := MustMakeTyped[string](c, "name")
	if v != "hello" {
		t.Fatalf("expected 'hello', got %q", v)
	}
}

func TestMustMakeTypedPanics(t *testing.T) {
	c := New()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustMakeTyped should panic on missing binding")
		}
	}()
	MustMakeTyped[string](c, "nonexistent")
}

type myStruct struct{ Val int }

func TestMakeTypedStruct(t *testing.T) {
	c := New()
	c.Singleton("obj", func(_ Container) (any, error) {
		return &myStruct{Val: 99}, nil
	})

	v, err := MakeTyped[*myStruct](c, "obj")
	if err != nil {
		t.Fatal(err)
	}
	if v.Val != 99 {
		t.Fatalf("expected 99, got %d", v.Val)
	}
}

func TestMakeTypedInterface(t *testing.T) {
	c := New()
	c.Instance("driver", &testDriver{name: "test"})

	v, err := MakeTyped[Driver](c, "driver")
	if err != nil {
		t.Fatal(err)
	}
	if v.Name() != "test" {
		t.Fatalf("expected 'test', got %q", v.Name())
	}
}
