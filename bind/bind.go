// Package bind declares what crystalline should expose to JavaScript.
//
// It is deliberately free of reflect, and of any dependency that pulls reflect
// in, because a manifest written against it is compiled into the wasm binary.
// Importing the root crystalline package from a manifest would drag the whole
// reflection runtime back into the build.
//
// A manifest is an ordinary function, marked so the generator can find it:
//
//	//crystalline:exports
//	func Exports(r bind.Registry) {
//		r.Func(api.Greet)
//		r.Func(api.Load, bind.AsPromise())
//		r.Value("Version", api.Version)
//	}
//
// The generator reads the function to learn the types involved; the function
// itself runs at start-up to supply the values. Ordinary Go around the calls —
// locals, loops, conversions — is left alone, since only the static type of
// each argument is needed.
package bind

// Option customises how a single entity is exposed.
type Option func(*Options)

// Options is the resolved form of the options passed to a Registry call. It is
// exported so that generated code can accept it.
type Options struct {
	// Promise makes the exposed function return a JS Promise.
	Promise bool

	// Namespace overrides the JS namespace, which otherwise follows the Go
	// package the entity comes from.
	Namespace string
}

// AsPromise makes an exposed function return a JS Promise, running the Go call
// on its own goroutine so it does not block the JS event loop.
func AsPromise() Option {
	return func(o *Options) {
		o.Promise = true
	}
}

// InNamespace overrides the JS namespace an entity is placed under.
func InNamespace(name string) Option {
	return func(o *Options) {
		o.Namespace = name
	}
}

// Resolve folds a list of options into their resolved form.
func Resolve(opts []Option) Options {
	var resolved Options

	for _, opt := range opts {
		opt(&resolved)
	}

	return resolved
}

// Registry receives the declarations a manifest makes.
//
// The implementation is generated: at run time it publishes each entity into
// the JS object graph. Names passed to Value, Ignore and Promise must be string
// literals, so that the generator can resolve them without executing anything.
type Registry interface {
	// Func exposes a Go function under its own package and name.
	Func(fn any, opts ...Option)

	// Value exposes a value under the given name. A value carries no name at
	// run time, so one has to be supplied.
	Value(name string, value any, opts ...Option)

	// Type declares a type so that it appears in the generated declarations
	// even when no exposed entity mentions it. The argument is only read for
	// its type; a zero value is the usual thing to pass.
	Type(zero any)

	// Plain marshals the given type as ordinary JavaScript data rather than as
	// a live wrapper.
	//
	// A wrapper reads and writes through to the Go value, which costs a call
	// across the boundary per field access and a slot in the bridge per field
	// and method. For a result that is only read, that is a great deal of
	// machinery to pay for a snapshot. Plain data is converted once, has no
	// methods and cannot be written back.
	//
	// It applies to the whole type wherever it appears, and to every struct
	// reachable from it: a plain value cannot contain a live one.
	Plain(zero any)

	// Marshal maps a type onto a JavaScript counterpart, using a pair of
	// ordinary Go functions.
	//
	// The signatures carry the declaration: func(T) X says T crosses as
	// whatever X does, and func(X) (T, error) says how it comes back and that
	// it may refuse. Both are checked when generating.
	//
	// It suits a type whose value is not its fields. A struct is otherwise
	// bound as a live view of what it exports, which says nothing useful about
	// a colour, an identifier or an amount of money.
	//
	// The mapping applies wherever the type appears.
	Marshal(to any, from any)

	// Ignore keeps a method off the JS surface of the given type.
	Ignore(zero any, method string)

	// Promise makes a method of the given type return a JS Promise.
	Promise(zero any, method string)
}
