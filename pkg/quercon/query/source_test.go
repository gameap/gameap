package query

// source.go wraps rumblefrog/go-a2s; only failure paths are unit-testable.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuerySource_QueryInfoFailure exercises the path where a2s.NewClient
// succeeds (the address parses) but QueryInfo fails because nothing answers
// on the unreachable port. This branch returns the partially-initialized
// Result struct alongside the wrapped error.
func TestQuerySource_QueryInfoFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := querySource(ctx, "127.0.0.1", 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query info", "error must be wrapped with QueryInfo context")

	require.NotNil(t, result, "result is constructed before QueryInfo and must be returned on info failure")
	assert.False(t, result.Online, "Online must be false when info query fails")
	assert.False(t, result.QueryTime.IsZero(), "QueryTime is set before the info call and must be populated")
	assert.Empty(t, result.Name, "Name must remain empty when info query fails")
	assert.Empty(t, result.Map, "Map must remain empty when info query fails")
	assert.Equal(t, 0, result.PlayersNum, "PlayersNum must remain zero when info query fails")
	assert.Equal(t, 0, result.MaxPlayersNum, "MaxPlayersNum must remain zero when info query fails")
	assert.Empty(t, result.Players, "Players must remain empty when info query fails")
}

// TestQuerySource_NewClientFailure exercises the early-return path where
// a2s.NewClient itself fails. A negative port makes net.DialTimeout reject
// the address synchronously with "invalid port" before any I/O, regardless
// of platform (some platforms accept ":0" and surface the failure later
// at QueryInfo time instead). On this early branch the function returns
// (nil, wrapped error).
func TestQuerySource_NewClientFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := querySource(ctx, "127.0.0.1", -1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create a2s client", "early failure must surface the NewClient wrap")
	assert.Nil(t, result, "early NewClient failure must return a nil result")
}

// TestQuerySource_ContextIgnored documents that the wrapper currently does
// not propagate the context to the underlying a2s client (the parameter is
// blanked in the signature). A pre-cancelled context must therefore still
// reach the QueryInfo failure path rather than aborting early.
func TestQuerySource_ContextIgnored(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := querySource(ctx, "127.0.0.1", 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query info", "context cancellation must not short-circuit since ctx is ignored")
	require.NotNil(t, result)
	assert.False(t, result.Online)
}
