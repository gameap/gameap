package interceptors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecoveryInterceptor_Unary_RecoversPanic(t *testing.T) {
	t.Parallel()
	// ARRANGE
	interceptor := NewRecoveryInterceptor(discardLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/Boom"}
	handler := func(_ context.Context, _ any) (any, error) {
		panic("boom")
	}

	// ACT
	resp, err := func() (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic propagated past interceptor: %v", r)
			}
		}()

		return interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)
	}()

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err), "panic must be converted to Internal")
	assert.Contains(t, err.Error(), "internal server error")
	assert.Nil(t, resp, "no response when handler panics")
}

func TestRecoveryInterceptor_Unary_NoPanic_PassesThrough(t *testing.T) {
	t.Parallel()
	// ARRANGE
	interceptor := NewRecoveryInterceptor(discardLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/Ok"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	// ACT
	resp, err := interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestRecoveryInterceptor_Stream_RecoversPanic(t *testing.T) {
	t.Parallel()
	// ARRANGE
	interceptor := NewRecoveryInterceptor(discardLogger())
	info := &grpc.StreamServerInfo{FullMethod: "/gameap.DaemonGateway/StreamBoom"}
	handler := func(_ any, _ grpc.ServerStream) error {
		panic("boom")
	}
	ss := &stubStream{ctx: context.Background()}

	// ACT
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic propagated past interceptor: %v", r)
			}
		}()

		return interceptor.StreamServerInterceptor()(nil, ss, info, handler)
	}()

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err), "panic must be converted to Internal")
	assert.Contains(t, err.Error(), "internal server error")
}

func TestRecoveryInterceptor_Stream_NoPanic_PassesThrough(t *testing.T) {
	t.Parallel()
	// ARRANGE
	interceptor := NewRecoveryInterceptor(discardLogger())
	info := &grpc.StreamServerInfo{FullMethod: "/gameap.DaemonGateway/StreamOk"}
	handler := func(_ any, _ grpc.ServerStream) error {
		return nil
	}
	ss := &stubStream{ctx: context.Background()}

	// ACT
	err := interceptor.StreamServerInterceptor()(nil, ss, info, handler)

	// ASSERT
	require.NoError(t, err)
}

func TestNewRecoveryInterceptor_NilLoggerUsesDefault(t *testing.T) {
	t.Parallel()
	// ARRANGE & ACT
	interceptor := NewRecoveryInterceptor(nil)

	// ASSERT
	require.NotNil(t, interceptor)
	require.NotNil(t, interceptor.UnaryServerInterceptor())
	require.NotNil(t, interceptor.StreamServerInterceptor())

	// Sanity check: panic is still recovered with the default logger.
	info := &grpc.UnaryServerInfo{FullMethod: "/x/y"}
	handler := func(_ context.Context, _ any) (any, error) { panic("boom") }
	resp, err := interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Nil(t, resp)
}
