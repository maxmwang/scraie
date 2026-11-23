package option

import "fmt"

type Option[T any] struct {
	isSome bool
	v      T
}

func Some[T any](v T) Option[T] {
	return Option[T]{
		isSome: true,
		v:      v,
	}
}

func None[T any]() Option[T] {
	return Option[T]{
		isSome: false,
	}
}

func SomeIf[T any](p bool, v T) Option[T] {
	if p {
		return Some(v)
	}
	return None[T]()
}

func SomeIfPtr[T any](p bool, v *T) Option[T] {
	if p {
		return Some(*v)
	}
	return None[T]()
}

func (o Option[T]) IsSome() bool {
	return o.isSome
}

// Value returns (v, true) if isSome, else returns the zero value and false.
func (o Option[T]) Value() (T, bool) {
	if o.isSome {
		return o.v, true
	}
	return *new(T), false
}

func (o Option[T]) ValueOrDefault(defaultValue T) T {
	if o.isSome {
		return o.v
	}
	return defaultValue
}

func (o Option[T]) String() string {
	if o.isSome {
		return fmt.Sprintf("some(%v)", o.v)
	}
	return "none"
}
