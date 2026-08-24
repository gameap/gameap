package plugin

import (
	"slices"

	"github.com/tetratelabs/wazero"
)

// recordingRuntime notes the name of every host module instantiated on the
// runtime handed to the host libraries, so a plugin can be told which
// modules the panel provides to it (gameap-host GetHostInfo). WASI and the
// AssemblyScript env module are instantiated on the raw runtime and are
// therefore not recorded.
type recordingRuntime struct {
	wazero.Runtime

	names map[string]struct{}
}

func recordHostModules(r wazero.Runtime) *recordingRuntime {
	return &recordingRuntime{Runtime: r, names: make(map[string]struct{})}
}

func (r *recordingRuntime) NewHostModuleBuilder(moduleName string) wazero.HostModuleBuilder {
	r.names[moduleName] = struct{}{}

	return r.Runtime.NewHostModuleBuilder(moduleName)
}

// Modules lists the recorded host module names, sorted.
func (r *recordingRuntime) Modules() []string {
	modules := make([]string, 0, len(r.names))
	for name := range r.names {
		modules = append(modules, name)
	}

	slices.Sort(modules)

	return modules
}
