package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_duplicate_returns_already_loaded(t *testing.T) {
	loadSharedServerLoggerWASM(t)
	wasmBytes, err := decompressServerLoggerWASM()
	require.NoError(t, err)

	before := len(sharedManager.GetPlugins())

	_, err = sharedManager.Load(context.Background(), wasmBytes, nil, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginAlreadyLoaded)
	require.Len(t, sharedManager.GetPlugins(), before, "duplicate load must not replace the registered plugin")
}

func TestLoadTransient_does_not_register_plugin(t *testing.T) {
	shared := loadSharedServerLoggerWASM(t)
	wasmBytes, err := decompressServerLoggerWASM()
	require.NoError(t, err)

	before := len(sharedManager.GetPlugins())

	transient, err := sharedManager.LoadTransient(context.Background(), wasmBytes, nil, 0)
	require.NoError(t, err)
	require.Len(t, sharedManager.GetPlugins(), before, "transient plugin must not be registered")

	registered, ok := sharedManager.GetPlugin(transient.Info.Id)
	require.True(t, ok)
	assert.Same(t, shared, registered, "registry must still point to the originally loaded plugin")

	require.NoError(t, transient.Close(context.Background()))

	info, err := shared.Instance.GetInfo(context.Background(), &proto.GetInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, transient.Info.Id, info.Id, "closing the transient instance must not affect the registered plugin")
}
