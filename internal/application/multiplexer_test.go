package application

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestNewMultiplexedServer(t *testing.T) {
	t.Parallel()

	t.Run("plaintext_listener_is_bound_on_loopback", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		grpcSrv := grpc.NewServer()
		t.Cleanup(grpcSrv.Stop)

		// ACT
		srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
			Address:    "127.0.0.1:0",
			GRPCServer: grpcSrv,
			HTTPServer: &http.Server{ReadHeaderTimeout: time.Second},
		})

		// ASSERT
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NotNil(t, srv.Address())
		_, ok := srv.Address().(*net.TCPAddr)
		assert.True(t, ok, "address should be TCP")
	})

	t.Run("tls_listener_wraps_with_tls_config", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		grpcSrv := grpc.NewServer()
		t.Cleanup(grpcSrv.Stop)

		tlsCfg := newSelfSignedTLSConfig(t)

		// ACT
		srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
			Address:    "127.0.0.1:0",
			TLSConfig:  tlsCfg,
			GRPCServer: grpcSrv,
			HTTPServer: &http.Server{ReadHeaderTimeout: time.Second},
		})

		// ASSERT
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })
		require.NotNil(t, srv.Address())
	})

	t.Run("nil_logger_falls_back_to_default", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		grpcSrv := grpc.NewServer()
		t.Cleanup(grpcSrv.Stop)

		// ACT
		srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
			Address:    "127.0.0.1:0",
			GRPCServer: grpcSrv,
			HTTPServer: &http.Server{ReadHeaderTimeout: time.Second},
			Logger:     nil,
		})

		// ASSERT
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })
		assert.NotNil(t, srv.logger)
	})

	t.Run("invalid_address_returns_listener_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := context.Background()
		grpcSrv := grpc.NewServer()
		t.Cleanup(grpcSrv.Stop)

		// ACT
		srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
			Address:    "127.0.0.1:invalid-port",
			GRPCServer: grpcSrv,
			HTTPServer: &http.Server{ReadHeaderTimeout: time.Second},
		})

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, srv)
	})
}

func TestMultiplexedServer_Address(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	grpcSrv := grpc.NewServer()
	t.Cleanup(grpcSrv.Stop)

	srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
		Address:    "127.0.0.1:0",
		GRPCServer: grpcSrv,
		HTTPServer: &http.Server{ReadHeaderTimeout: time.Second},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// ACT
	addr := srv.Address()

	// ASSERT
	require.NotNil(t, addr)
	tcpAddr, ok := addr.(*net.TCPAddr)
	require.True(t, ok)
	assert.True(t, tcpAddr.Port > 0, "port should be assigned")
	assert.Equal(t, "127.0.0.1", tcpAddr.IP.String())
}

func TestMultiplexedServer_Close_BeforeServe(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	grpcSrv := grpc.NewServer()
	t.Cleanup(grpcSrv.Stop)

	srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
		Address:    "127.0.0.1:0",
		GRPCServer: grpcSrv,
		HTTPServer: &http.Server{ReadHeaderTimeout: time.Second},
	})
	require.NoError(t, err)

	// ACT
	closeErr := srv.Close()

	// ASSERT
	require.NoError(t, closeErr)

	closeAgain := srv.Close()
	assert.Error(t, closeAgain, "second close should fail because listener already closed")
}

func TestMultiplexedServer_Serve_RoutesGRPCAndHTTP(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	grpcSrv := grpc.NewServer()
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("test.Service", healthpb.HealthCheckResponse_SERVING)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	httpSrv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}

	srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
		Address:    "127.0.0.1:0",
		GRPCServer: grpcSrv,
		HTTPServer: httpSrv,
		Logger:     slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)

	addr := srv.Address().String()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- srv.Serve(ctx)
	}()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()

		return true
	}, 2*time.Second, 25*time.Millisecond, "multiplexer should accept connections")

	// ACT 1: HTTP request through multiplexer
	httpClient := &http.Client{Timeout: 2 * time.Second}
	resp, httpErr := httpClient.Get("http://" + addr + "/ping")

	// ASSERT 1
	require.NoError(t, httpErr)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "pong", string(body))

	// ACT 2: gRPC request through the same multiplexer
	grpcConn, dialErr := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, dialErr)
	t.Cleanup(func() { _ = grpcConn.Close() })

	healthClient := healthpb.NewHealthClient(grpcConn)

	rpcCtx, rpcCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(rpcCancel)

	hr, rpcErr := healthClient.Check(rpcCtx, &healthpb.HealthCheckRequest{Service: "test.Service"})

	// ASSERT 2
	require.NoError(t, rpcErr)
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, hr.GetStatus())

	// ACT 3: shutdown via context cancellation
	cancel()

	// ASSERT 3: Serve unwinds within the timeout. The current production code does not
	// filter cmux.ErrServerClosed nor "use of closed network connection" from the listener,
	// so an error is allowed here — what matters is that Serve actually exits.
	select {
	case <-serveErrCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not exit after context cancellation")
	}
}

func TestMultiplexedServer_Serve_TLS(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	tlsCfg := newSelfSignedTLSConfig(t)

	grpcSrv := grpc.NewServer()
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("test.Service", healthpb.HealthCheckResponse_SERVING)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	httpSrv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}

	srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
		Address:    "127.0.0.1:0",
		TLSConfig:  tlsCfg,
		GRPCServer: grpcSrv,
		HTTPServer: httpSrv,
		Logger:     slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)

	addr := srv.Address().String()

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- srv.Serve(ctx)
	}()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()

		return true
	}, 2*time.Second, 25*time.Millisecond, "TLS multiplexer should accept connections")

	clientTLSCfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}

	// ACT: HTTPS request via TLS-wrapped multiplexer
	httpClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: clientTLSCfg},
		Timeout:   2 * time.Second,
	}
	resp, httpErr := httpClient.Get("https://" + addr + "/ping")

	// ASSERT
	require.NoError(t, httpErr)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "pong", string(body))

	cancel()

	select {
	case <-serveErrCh:
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not exit after context cancellation")
	}
}

func TestMultiplexedServer_Serve_FiltersGRPCStoppedError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	grpcSrv := grpc.NewServer()
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)

	httpSrv := &http.Server{
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}

	srv, err := NewMultiplexedServer(ctx, &MultiplexerConfig{
		Address:    "127.0.0.1:0",
		GRPCServer: grpcSrv,
		HTTPServer: httpSrv,
		Logger:     slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	addr := srv.Address().String()

	var wg sync.WaitGroup
	wg.Add(1)
	var serveErr error
	go func() {
		defer wg.Done()
		serveErr = srv.Serve(ctx)
	}()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()

		return true
	}, 2*time.Second, 25*time.Millisecond, "multiplexer should accept connections")

	// ACT
	cancel()
	wg.Wait()

	// ASSERT — grpc.ErrServerStopped is explicitly swallowed inside the gRPC goroutine.
	// The Serve return may carry a different error (e.g. cmux server closed) but
	// grpc.ErrServerStopped itself must never surface from Serve.
	if serveErr != nil {
		assert.False(t, errors.Is(serveErr, grpc.ErrServerStopped),
			"grpc.ErrServerStopped must be swallowed by the gRPC goroutine")
	}
}

func newSelfSignedTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}
