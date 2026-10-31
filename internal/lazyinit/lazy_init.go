package lazyinit

import "sync"

// LazyInit is lazy value initialization type.
// It helps construct object on access and only once.
type LazyInit struct {
	// constructor is an object construction function.
	constructor func() (any, error)

	// once is a synchronization object to make initialization only once.
	once sync.Once

	// value is a lazy initialized value variable.
	value any

	// err is an error which is got on initialization.
	err error
}

// NewLazyInit creates a new lazy initialization variable.
// It takes a constructor function to make an object.
func NewLazyInit(constructor func() (any, error)) *LazyInit {
	return &LazyInit{constructor: constructor}
}

// GetValue returns initialized value or error if something wrong happened.
func (l *LazyInit) GetValue() (any, error) {
	l.once.Do(func() {
		l.value, l.err = l.constructor()
	})
	return l.value, l.err
}
