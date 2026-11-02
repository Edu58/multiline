package multiline

// Middleware wraps handler execution, for example to add metrics or tracing.
// Wrappers must be safe for concurrent use and should propagate the context.
type Middleware func(Handler) Handler

// Chain wraps h with middleware in declaration order: the first wrapper is
// outermost. Panics from middleware are contained by the scheduler's handler
// boundary, just like panics from h. Inputs must be non-nil.
func Chain(h Handler, middleware ...Middleware) Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
