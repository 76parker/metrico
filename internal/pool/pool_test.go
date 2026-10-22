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

type testResettableValue int

func (testResettableValue) Reset() {}

func TestPool_GetWithoutFactoryReturnsZeroValue(t *testing.T) {
	tests := []struct {
		name     string
		get      func() any
		expected any
	}{
		{
			name: "pointer",
			get: func() any {
				return New[*testValue](nil).Get()
			},
			expected: (*testValue)(nil),
		},
		{
			name: "value",
			get: func() any {
				return New[testResettableValue](nil).Get()
			},
			expected: testResettableValue(0),
		},
		{
			name: "interface",
			get: func() any {
				return New[Resettable](nil).Get()
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.get(); got != testCase.expected {
				t.Errorf("Get() = %#v, want %#v", got, testCase.expected)
			}
		})
	}
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

func TestPool_GetReturnsPooledOrNewValue(t *testing.T) {
	factoryCalls := 0
	value := &testValue{value: 42}
	factoryValue := &testValue{}
	values := New(func() *testValue {
		factoryCalls++
		return factoryValue
	})

	values.Put(value)
	got := values.Get()

	// sync.Pool may discard a stored value at any time, including in race builds.
	if got != value && got != factoryValue {
		t.Fatal("Get() returned neither the pooled value nor the factory value")
	}
	if got.value != 0 {
		t.Fatalf("Get() returned value %d, want 0", got.value)
	}
	expectedFactoryCalls := 0
	if got == factoryValue {
		expectedFactoryCalls = 1
	}
	if factoryCalls != expectedFactoryCalls {
		t.Fatalf("factory called %d times, want %d", factoryCalls, expectedFactoryCalls)
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
