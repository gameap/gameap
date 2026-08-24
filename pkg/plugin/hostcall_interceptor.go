package plugin

import (
	"context"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// observedRuntime decorates the runtime handed to host libraries so every
// host function they export is timed and counted for the Observer. The
// generated SDK glue registers functions through
// NewHostModuleBuilder().NewFunctionBuilder().WithGoModuleFunction(...).Export(name),
// which is the only seam the host has into guest→host calls, so wrapping it
// covers every module without touching the libraries themselves.
type observedRuntime struct {
	wazero.Runtime

	observer Observer
	pluginID uint64
}

// observeHostCalls wraps r unless there is nothing to report to.
func observeHostCalls(r wazero.Runtime, observer Observer, pluginID uint64) wazero.Runtime {
	if observer == nil {
		return r
	}

	if _, nop := observer.(NopObserver); nop {
		return r
	}

	return &observedRuntime{Runtime: r, observer: observer, pluginID: pluginID}
}

func (r *observedRuntime) NewHostModuleBuilder(moduleName string) wazero.HostModuleBuilder {
	return &observedModuleBuilder{
		HostModuleBuilder: r.Runtime.NewHostModuleBuilder(moduleName),
		runtime:           r,
		module:            moduleName,
	}
}

type observedModuleBuilder struct {
	wazero.HostModuleBuilder

	runtime *observedRuntime
	module  string
}

func (b *observedModuleBuilder) NewFunctionBuilder() wazero.HostFunctionBuilder {
	return &observedFunctionBuilder{
		HostFunctionBuilder: b.HostModuleBuilder.NewFunctionBuilder(),
		parent:              b,
	}
}

// observedFunctionBuilder defers the real registration to Export, the first
// point where the function's name is known.
type observedFunctionBuilder struct {
	wazero.HostFunctionBuilder

	parent   *observedModuleBuilder
	moduleFn api.GoModuleFunction
	goFn     api.GoFunction
	params   []api.ValueType
	results  []api.ValueType
}

func (b *observedFunctionBuilder) WithGoModuleFunction(
	fn api.GoModuleFunction, params, results []api.ValueType,
) wazero.HostFunctionBuilder {
	b.moduleFn, b.goFn = fn, nil
	b.params, b.results = params, results

	return b
}

func (b *observedFunctionBuilder) WithGoFunction(
	fn api.GoFunction, params, results []api.ValueType,
) wazero.HostFunctionBuilder {
	b.goFn, b.moduleFn = fn, nil
	b.params, b.results = params, results

	return b
}

// WithFunc registers a reflective Go function; those are not timed (the
// SDK glue never uses them).
func (b *observedFunctionBuilder) WithFunc(fn any) wazero.HostFunctionBuilder {
	b.moduleFn, b.goFn = nil, nil
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithFunc(fn)

	return b
}

func (b *observedFunctionBuilder) WithName(name string) wazero.HostFunctionBuilder {
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithName(name)

	return b
}

func (b *observedFunctionBuilder) WithParameterNames(names ...string) wazero.HostFunctionBuilder {
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithParameterNames(names...)

	return b
}

func (b *observedFunctionBuilder) WithResultNames(names ...string) wazero.HostFunctionBuilder {
	b.HostFunctionBuilder = b.HostFunctionBuilder.WithResultNames(names...)

	return b
}

func (b *observedFunctionBuilder) Export(name string) wazero.HostModuleBuilder {
	observer := b.parent.runtime.observer
	pluginID := b.parent.runtime.pluginID
	module := b.parent.module

	switch {
	case b.moduleFn != nil:
		fn := b.moduleFn
		b.HostFunctionBuilder = b.HostFunctionBuilder.WithGoModuleFunction(
			api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
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
