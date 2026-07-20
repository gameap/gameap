package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

var (
	errTestHostLib        = errors.New("simulated host library instantiate failure")
	errTestHostLibFactory = errors.New("simulated factory host library instantiate failure")
)

// emptyWASMModule is a valid module without any sections: it compiles and
// instantiates (missing start functions are skipped), then fails API version
// verification because it exports nothing.
var emptyWASMModule = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

// wrongAPIVersionWASMModule exports plugin_service_api_version returning 999,
// which does not match the host API version.
var wrongAPIVersionWASMModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f, // type: func () -> i32
	0x03, 0x02, 0x01, 0x00, // func: type 0
	0x07, 0x1e, 0x01, 0x1a, // export: 1 entry, name len 26
	'p', 'l', 'u', 'g', 'i', 'n', '_', 's', 'e', 'r', 'v', 'i', 'c', 'e', '_', 'a', 'p', 'i', '_', 'v', 'e', 'r', 's', 'i', 'o', 'n',
	0x00, 0x00, // kind func, index 0
	0x0a, 0x07, 0x01, 0x05, 0x00, 0x41, 0xe7, 0x07, 0x0b, // code: i32.const 999, end
}

// trapAPIVersionWASMModule exports plugin_service_api_version whose body is
// unreachable, so calling it traps.
var trapAPIVersionWASMModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f, // type: func () -> i32
	0x03, 0x02, 0x01, 0x00, // func: type 0
	0x07, 0x1e, 0x01, 0x1a, // export: 1 entry, name len 26
	'p', 'l', 'u', 'g', 'i', 'n', '_', 's', 'e', 'r', 'v', 'i', 'c', 'e', '_', 'a', 'p', 'i', '_', 'v', 'e', 'r', 's', 'i', 'o', 'n',
	0x00, 0x00, // kind func, index 0
	0x0a, 0x05, 0x01, 0x03, 0x00, 0x00, 0x0b, // code: unreachable, end
}

// unknownImportWASMModule imports a function from a module that does not
// exist, so instantiation fails.
var unknownImportWASMModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
	0x01, 0x04, 0x01, 0x60, 0x00, 0x00, // type: func () -> ()
	0x02, 0x10, 0x01, // import: 1 entry
	0x07, 'u', 'n', 'k', 'n', 'o', 'w', 'n', // module "unknown"
	0x04, 'f', 'u', 'n', 'c', // field "func"
	0x00, 0x00, // kind func, type 0
}

type failingHostLibFactory struct{ err error }

func (f failingHostLibFactory) Create(uint64) HostLibrary {
	return hostLibFunc(func(context.Context, wazero.Runtime) error {
		return f.err
	})
}

func TestLoadTransient_InvalidWASM(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), []byte("not a wasm module"), nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize runtime", "error message mismatch")
	assert.Contains(t, err.Error(), "failed to compile WASM module", "error message mismatch")
	assert.Nil(t, plugin)
}

func TestLoadTransient_EmptyModule(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), emptyWASMModule, nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExportNotFound, "a module without exports must fail API version verification")
	assert.Nil(t, plugin)
}

func TestLoadTransient_APIVersionMismatch(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), wrongAPIVersionWASMModule, nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAPIVersionMismatch, "a module reporting a foreign API version must be rejected")
	assert.Nil(t, plugin)
}

func TestLoadTransient_APIVersionCallTraps(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), trapAPIVersionWASMModule, nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call api_version", "error message mismatch")
	assert.Nil(t, plugin)
}

func TestLoadTransient_UnknownImport(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), unknownImportWASMModule, nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to instantiate module", "error message mismatch")
	assert.Nil(t, plugin)
}

func TestLoadTransient_HostLibraryInstantiateError(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{
		Libraries: []HostLibrary{
			hostLibFunc(func(context.Context, wazero.Runtime) error {
				return errTestHostLib
			}),
		},
	})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), emptyWASMModule, nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to instantiate host library", "error message mismatch")
	assert.ErrorIs(t, err, errTestHostLib)
	assert.Nil(t, plugin)
}

func TestLoadTransient_HostLibraryFactoryInstantiateError(t *testing.T) {
	// ARRANGE
	manager := NewManager(ManagerConfig{
		LibraryFactories: []HostLibraryFactory{
			failingHostLibFactory{err: errTestHostLibFactory},
		},
	})

	// ACT
	plugin, err := manager.LoadTransient(context.Background(), emptyWASMModule, nil, 0)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to instantiate factory host library", "error message mismatch")
	assert.ErrorIs(t, err, errTestHostLibFactory)
	assert.Nil(t, plugin)
}
