package ptr

// Of Returns a pointer to the given type T
func Of[T any](t T) *T {
	return &t
}
