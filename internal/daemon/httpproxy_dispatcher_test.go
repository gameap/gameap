package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type httpProxyDispatcherTestSetup struct {
	dispatcher HTTPProxyDispatcher
	gateway    *fakeHTTPProxyGateway
	registry   *fakeConnectionChecker
	storage    *files.InMemoryFileManager
	pubsub     pubsub.PubSub
	instanceID string
}

func setupHTTPProxyDispatcher(t *testing.T) *httpProxyDispatcherTestSetup {
	t.Helper()

	gateway := &fakeHTTPProxyGateway{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	logger := slog.New(slog.DiscardHandler)
	instanceID := testInstanceID

	dispatcher := NewHTTPProxyDispatcher(ps, gateway, registry, storage, instanceID, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, dispatcher.Start(ctx))

	return &httpProxyDispatcherTestSetup{
		dispatcher: dispatcher,
		gateway:    gateway,
		registry:   registry,
		storage:    storage,
		pubsub:     ps,
		instanceID: instanceID,
	}
}

func TestHTTPProxyDispatcher_DispatchHTTPProxy_Success_Inline(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	const nodeID uint64 = 7
	s.registry.setConnected(nodeID, true)

	var capturedURL string
	s.gateway.requestHTTPProxy = func(_ context.Context, _ uint64, req *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error) {
		capturedURL = req.Url

		return &proto.HTTPProxyResponse{Success: true, StatusCode: 200, Body: []byte("ok")}, nil
	}

	// ACT
	resp, err := s.dispatcher.DispatchHTTPProxy(
		testContext(t), nodeID, &proto.HTTPProxyRequest{Method: "GET", Url: "http://localhost/health"},
	)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, int32(200), resp.StatusCode)
	assert.Equal(t, "ok", string(resp.Body))
	assert.Equal(t, "http://localhost/health", capturedURL, "request must reach gateway via pubsub payload")
}

func TestHTTPProxyDispatcher_LargeResponse_StoredAndResolved(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	// Body large enough that the marshaled payload exceeds storageThreshold (4KB),
	// forcing the dispatcher to spill the response to shared storage.
	largeBody := bytes.Repeat([]byte("X"), storageThreshold+1024)
	s.gateway.requestHTTPProxy = func(_ context.Context, _ uint64, _ *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error) {
		return &proto.HTTPProxyResponse{Success: true, StatusCode: 200, Body: largeBody}, nil
	}

	// ACT
	resp, err := s.dispatcher.DispatchHTTPProxy(
		testContext(t), nodeID, &proto.HTTPProxyRequest{Method: "GET", Url: "http://localhost/big"},
	)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	require.Len(t, resp.Body, len(largeBody), "large body must round-trip through storage intact")
	assert.Equal(t, largeBody, resp.Body)

	// resolveResponseData must delete the spilled object after reading it.
	remaining, listErr := s.storage.List(testContext(t), httpProxyStoragePrefix)
	require.NoError(t, listErr)
	assert.Empty(t, remaining, "spilled storage object must be deleted after the response is resolved")
}

func TestHTTPProxyDispatcher_resolveResponseData_StorageReadError(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	d := s.dispatcher.(*httpProxyDispatcher)

	// ACT
	data, err := d.resolveResponseData(testContext(t), &messages.DaemonHTTPProxyResponsePayload{
		StoragePath: httpProxyStoragePrefix + "missing",
	})

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "read from storage", "missing storage object must surface a read error")
}

func TestHTTPProxyDispatcher_resolveResponseData_InlineData(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	d := s.dispatcher.(*httpProxyDispatcher)

	// ACT
	data, err := d.resolveResponseData(testContext(t), &messages.DaemonHTTPProxyResponsePayload{
		Data: []byte("inline"),
	})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, []byte("inline"), data, "inline data must be returned without touching storage")
}

func TestHTTPProxyDispatcher_DispatchHTTPProxy_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	const nodeID uint64 = 9
	s.registry.setConnected(nodeID, true)

	s.gateway.requestHTTPProxy = func(_ context.Context, _ uint64, _ *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error) {
		return nil, errors.New("gateway boom")
	}

	// ACT
	resp, err := s.dispatcher.DispatchHTTPProxy(testContext(t), nodeID, &proto.HTTPProxyRequest{Url: "http://x"})

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "gateway boom", "gateway error must propagate via pubsub response")
}

func TestHTTPProxyDispatcher_executeRequest_UnmarshalError(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	d := s.dispatcher.(*httpProxyDispatcher)

	// ACT
	resp := d.executeRequest(testContext(t), messages.DaemonHTTPProxyRequestPayload{
		NodeID:    1,
		RequestID: "req-test",
		Data:      []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9},
	})

	// ASSERT
	assert.NotEmpty(t, resp.Error, "malformed request payload must yield error response")
}

func TestHTTPProxyDispatcher_NotConnected_TimesOut(t *testing.T) {
	// ARRANGE
	s := setupHTTPProxyDispatcher(t)
	const nodeID uint64 = 99
	s.registry.setConnected(nodeID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// ACT
	resp, err := s.dispatcher.DispatchHTTPProxy(ctx, nodeID, &proto.HTTPProxyRequest{Url: "http://x"})

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "must surface context deadline when no daemon answers")
}

func TestNewHTTPProxyDispatcher_NilLoggerUsesDefault(t *testing.T) {
	// ARRANGE
	gateway := &fakeHTTPProxyGateway{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	// ACT
	dispatcher := NewHTTPProxyDispatcher(ps, gateway, registry, storage, "id", nil)

	// ASSERT
	require.NotNil(t, dispatcher)
}
