package plugin

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// hostCallDeadlineMargin is held back from the guest call deadline for every
// host function. A host operation that would run into the deadline returns
// early instead, so the guest gets control back and can answer before the
// runtime closes its module: the watchdog only marks the module closed,
// it cannot interrupt a host function blocked on the network or a daemon.
const hostCallDeadlineMargin = time.Second

// interceptedRuntime decorates the runtime handed to host libraries so every
// host function they export runs with the deadline margin and is timed and
// counted for the Observer. The generated SDK glue registers functions
// through NewHostModuleBuilder().NewFunctionBuilder().WithGoModuleFunction(...).Export(name),
// which is the only seam the host has into guest→host calls, so wrapping it
// covers every module without touching the libraries themselves.
type interceptedRuntime struct {
	wazero.Runtime

	observer Observer
	pluginID uint64
}

// interceptHostCalls wraps r for a registered plugin. A nil observer only
// disables the reporting; the deadline margin applies regardless.
func interceptHostCalls(r wazero.Runtime, observer Observer, pluginID uint64) wazero.Runtime {
	return &interceptedRuntime{Runtime: r, observer: observerOrNop(observer), pluginID: pluginID}
}

func (r *interceptedRuntime) NewHostModuleBuilder(moduleName string) wazero.HostModuleBuilder {
	return &interceptedModuleBuilder{
		HostModuleBuilder: r.Runtime.NewHostModuleBuilder(moduleName),
		runtime:           r,
		module:            moduleName,
	}
}

type interceptedModuleBuilder struct {
	wazero.HostModuleBuilder

	runtime *interceptedRuntime
	module  string
}

func (b *interceptedModuleBuilder) NewFunctionBuilder() wazero.HostFunctionBuilder {
	return &interceptedFunctionBuilder{
		HostFunctionBuilder: b.HostModuleBuilder.NewFunctionBuilder(),
		parent:              b,
	}
}

// interceptedFunctionBuilder defers the real registration to Export, the
// first point where the function's name is known.
type interceptedFunctionBuilder struct {
	wazero.HostFunctionBuilder

	parent   *interceptedModuleBuilder
	moduleFn api.GoModuleFunction
	goFn     api.GoFunction
	params   []api.ValueType
	results  []api.ValueType
}

func (b *interceptedFunctionBuilder) WithGoModuleFunction(
	fn api.GoModuleFunction, params, results []api.ValueType,
) wazero.HostFunctionBuilder {
	b.moduleFn, b.goFn = fn, nil
	b.params, b.results = params, results

	return b
}

func (b *interceptedFunctionBuilder) WithGoFunction(
	fn api.GoFunction, params, results []api.ValueType,
) wazero.HostFunctionBuilder {
	b.goFn, b.moduleFn = fn, nil
	b.params, b.results = params, results

	return b
}

// WithFunc registers a reflective Go function; those are neither timed nor
// given the deadline margin (the SDK glue never uses them).
func (b *interceptedFunctionBuilder) WithFunc(fn any) wazero.HostFunctionBuilder {
	b.moduleFn, b.goFn = nil, nil
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithFunc(fn)

	return b
}

func (b *interceptedFunctionBuilder) WithName(name string) wazero.HostFunctionBuilder {
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithName(name)

	return b
}

func (b *interceptedFunctionBuilder) WithParameterNames(names ...string) wazero.HostFunctionBuilder {
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithParameterNames(names...)

	return b
}

func (b *interceptedFunctionBuilder) WithResultNames(names ...string) wazero.HostFunctionBuilder {
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithResultNames(names...)

	return b
}

func (b *interceptedFunctionBuilder) Export(name string) wazero.HostModuleBuilder {
	observer := b.parent.runtime.observer
	pluginID := b.parent.runtime.pluginID
	module := b.parent.module

	switch {
	case b.moduleFn != nil:
		fn := b.moduleFn
		b.HostFunctionBuilder = b.HostFunctionBuilder.WithGoModuleFunction(
			api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
				ctx, cancel := hostCallContext(ctx)
				defer cancel()

				start := time.Now()
				panicked := true
				// Runs during panic unwinding too, so a host function that
				// panics (the glue does that on a Go error) is still counted.
				defer func() {
					observer.HostCall(pluginID, module, name, time.Since(start), panicked)
				}()

				fn.Call(ctx, mod, stack)
				panicked = false
			}), b.params, b.results)
	case b.goFn != nil:
		fn := b.goFn
		b.HostFunctionBuilder = b.HostFunctionBuilder.WithGoFunction(
			api.GoFunc(func(ctx context.Context, stack []uint64) {
				ctx, cancel := hostCallContext(ctx)
				defer cancel()

				start := time.Now()
				panicked := true
				defer func() {
					observer.HostCall(pluginID, module, name, time.Since(start), panicked)
				}()

				fn.Call(ctx, stack)
				panicked = false
			}), b.params, b.results)
	}

	b.HostFunctionBuilder.Export(name)

	return b.parent
}

// hostCallContext derives the context a host function runs with: the guest
// call's deadline less the margin. A call without a deadline is passed on
// as is.
func hostCallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx, func() {}
	}

	return context.WithDeadline(ctx, deadline.Add(-hostCallDeadlineMargin))
}
