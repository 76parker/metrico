// Package pool provides reusable pools for resettable values.
package pool

import "sync"

// Resettable is implemented by values that can clear their reusable state.
type Resettable interface {
	Reset()
}

// Pool stores reusable values and resets them before they are returned.
type Pool[T Resettable] struct {
	values sync.Pool
}

// New creates a pool that calls factory when no value is available.
func New[T Resettable](factory func() T) *Pool[T] {
	return &Pool[T]{
		values: sync.Pool{
			New: func() any {
				return factory()
			},
		},
	}
}

// Get returns a pooled value or creates one with the configured factory.
func (p *Pool[T]) Get() T {
	return p.values.Get().(T)
}

// Put resets value and makes it available for reuse.
func (p *Pool[T]) Put(value T) {
	value.Reset()
	p.values.Put(value)
}
