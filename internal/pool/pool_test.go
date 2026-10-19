package pool

import "testing"

type testValue struct {
	value  int
	resets int
}

func (v *testValue) Reset() {
	v.value = 0
	v.resets++
}

func TestPool_GetCreatesValueWhenEmpty(t *testing.T) {
	factoryCalls := 0
	value := &testValue{value: 42}
	values := New(func() *testValue {
		factoryCalls++
		return value
	})

	got := values.Get()

	if got != value {
		t.Fatal("Get() did not return the value created by the factory")
	}
	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, want 1", factoryCalls)
	}
}

func TestPool_GetReturnsPooledValue(t *testing.T) {
	factoryCalls := 0
	value := &testValue{value: 42}
	values := New(func() *testValue {
		factoryCalls++
		return &testValue{}
	})

	values.Put(value)
	got := values.Get()

	if got != value {
		t.Fatal("Get() did not return the pooled value")
	}
	if factoryCalls != 0 {
		t.Fatalf("factory called %d times, want 0", factoryCalls)
	}
}

func TestPool_PutResetsValue(t *testing.T) {
	values := New(func() *testValue { return &testValue{} })
	value := &testValue{value: 42}

	values.Put(value)

	if value.value != 0 {
		t.Fatalf("value after Put() = %d, want 0", value.value)
	}
	if value.resets != 1 {
		t.Fatalf("Reset() called %d times, want 1", value.resets)
	}
}
