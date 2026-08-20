// White-box tests for the bootstrap helpers in application.go. They live in
// package application so they can drive the unexported helpers directly and swap
// the package-level osExit seam.
//
// The failure-path helpers (startHTTPListener bind error, startHTTPSServer with
// no cert source, runWithGRPC bind error) all reach an osExit(1) call. Some of
// those call sites have no return after them, so the test stub MUST panic to
// stop execution before the helper dereferences a nil listener/cert. Tests that
// mutate the shared osExit global therefore never run with t.Parallel().
//
// Run() itself is not tested here: it installs OS signal handlers, runs
// migrations + seeding and blocks on a shutdown channel — that is an
// integration concern, not a unit one.

package application

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// errExitSentinel is the value the stubbed osExit panics with so a failure-path
// test can recover it and assert the bootstrap helper decided to terminate.
var errExitSentinel = errors.New("osExit called")

// stubFatalExit swaps the package-level osExit for a function that panics with
// errExitSentinel, restoring the original on cleanup. Because osExit is a shared
// global, callers must not use t.Parallel(). The stub panics (rather than just
// recording) so that helpers whose osExit(1) site has no trailing return stop
// before dereferencing a nil listener or certificate.
func stubFatalExit(t *testing.T) {
	t.Helper()

	orig := osExit
	osExit = func(int) { panic(errExitSentinel) }
	t.Cleanup(func() { osExit = orig })
}

// assertFatalExit runs fn and asserts it triggered the stubbed osExit (i.e. it
// panicked with errExitSentinel). Any other panic value is re-raised so a real
// bug is not silently swallowed.
func assertFatalExit(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		r := recover()
		require.NotNil(t, r, "the bootstrap helper was expected to call osExit but returned normally")
		err, ok := r.(error)
		if !ok || !errors.Is(err, errExitSentinel) {
			panic(r)
		}
	}()

	fn()
}

// teardownContainer registers a cleanup that cancels ctx and runs the
// container's full Shutdown so the HTTP/HTTPS/pub-sub goroutines started by the
// bootstrap helpers are drained inside the test.
//
// Shutdown invokes the RBAC shutdown hook (close of the permission-cache
// channel), and newWiredContainer also closes RBAC in its own cleanup; that
// close is not idempotent, so the harness cleanup would panic with
// "close of closed channel" on the second close. Because t.Cleanup runs LIFO
// and newWiredContainer registered its RBAC cleanup first, this cleanup runs
// before it: after Shutdown has closed RBAC we detach c.rbac so the harness
// guard (if c.rbac != nil) skips the redundant second close.
func teardownContainer(t *testing.T, c *Container, cancel context.CancelFunc) {
	t.Helper()

	t.Cleanup(func() {
		cancel()
		require.NoError(t, c.Shutdown(), "Shutdown must drain the bootstrap goroutines cleanly")
		c.rbac = nil
	})
}

// takenPort binds an ephemeral loopback TCP port, then closes it and returns the
// port number. Re-binding that port immediately afterwards is overwhelmingly
// likely to fail with EADDRINUSE only if something else grabs it; the helper
// keeps the listener open via the returned closer so the caller controls when
// the port is freed, which makes the "address already in use" path deterministic.
func takenPort(t *testing.T) (port int, release func()) {
	t.Helper()

	lis, err := net.Listen("tcp", net.JoinHostPort(loopbackBindIP, "0"))
	require.NoError(t, err)

	addr, ok := lis.Addr().(*net.TCPAddr)
	require.True(t, ok, "listener address must be a *net.TCPAddr")

	return addr.Port, func() { _ = lis.Close() }
}

func TestStartPubSub(t *testing.T) {
	t.Parallel()

	t.Run("starts_cache_invalidator_bridge_and_listener_without_panic", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t)
		ctx, cancel := context.WithCancel(context.Background())
		c.SetContext(ctx, cancel)
		teardownContainer(t, c, cancel)

		// ACT
		startPubSub(ctx, c)

		// ASSERT
		// startPubSub wires the cache invalidator + WS bridge synchronously and
		// launches the pub-sub listener goroutine; the unit of behaviour here is
		// that wiring the memory pub-sub graph completes without panicking.
		require.NotNil(t, c.PubSub(), "the pub-sub graph must be wired after startPubSub")
	})
}

func TestStartUploadJanitor(t *testing.T) {
	t.Parallel()

	t.Run("launches_janitor_goroutine_without_panic", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.Files.Upload.JanitorInterval = 10 * time.Millisecond
			cfg.Files.Upload.SessionTTL = 50 * time.Millisecond
		})
		ctx, cancel := context.WithCancel(context.Background())
		c.SetContext(ctx, cancel)
		teardownContainer(t, c, cancel)

		// ACT
		startUploadJanitor(ctx, c)

		// ASSERT
		require.NotNil(t, c.UploadJanitor(), "the janitor must be wired after startUploadJanitor")
	})
}

//nolint:paralleltest // a subtest stubs the shared osExit global
func TestStartHTTPListener(t *testing.T) {
	t.Run("binds_ephemeral_port_and_serves_without_exit", func(t *testing.T) {
		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.AuthService = authServicePaseto
			cfg.HTTPBindIP = loopbackBindIP
			cfg.HTTPPort = 0
		})
		ctx, cancel := context.WithCancel(context.Background())
		c.SetContext(ctx, cancel)
		teardownContainer(t, c, cancel)

		// ACT
		startHTTPListener(ctx, c.Config(), c)

		// ASSERT
		require.NotNil(t, c.HTTPServer(), "a successful bind must leave the HTTP server constructed")
	})

	t.Run("calls_os_exit_when_the_bind_address_is_already_in_use", func(t *testing.T) {
		// ARRANGE
		port, release := takenPort(t)
		t.Cleanup(release)

		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.AuthService = authServicePaseto
			cfg.HTTPBindIP = loopbackBindIP
			cfg.HTTPPort = uint16(port)
		})
		stubFatalExit(t)

		// ACT / ASSERT
		assertFatalExit(t, func() {
			startHTTPListener(context.Background(), c.Config(), c)
		})
	})
}

// TestStartHTTPSServer covers the HTTPS bootstrap helper.
// OWASP API8:2023 Security Misconfiguration / ASVS §9.1: TLS startup must abort
// (osExit) when no certificate source is configured rather than silently serving
// an unusable listener; with a real certificate source it must start serving.
//
//nolint:paralleltest // a subtest stubs the shared osExit global
func TestStartHTTPSServer(t *testing.T) {
	t.Run("calls_os_exit_when_no_certificate_source_is_configured", func(t *testing.T) {
		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.AuthService = authServicePaseto
		})
		require.Equal(t, config.CertSourceNone, c.Config().EffectiveCertSource(),
			"sanity: no cert source must select CertSourceNone")
		stubFatalExit(t)

		// ACT / ASSERT
		assertFatalExit(t, func() {
			startHTTPSServer(context.Background(), c.Config(), c)
		})
	})

	t.Run("starts_serving_with_an_inline_certificate_without_exit", func(t *testing.T) {
		// ARRANGE
		certPEM, keyPEM := buildSelfSignedPEM(t)
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.AuthService = authServicePaseto
			cfg.HTTPBindIP = loopbackBindIP
			cfg.HTTPSPort = 0
			cfg.TLS.Cert = string(certPEM)
			cfg.TLS.Key = string(keyPEM)
		})
		require.Equal(t, config.CertSourceInline, c.Config().EffectiveCertSource(),
			"sanity: inline cert + key must select the inline source")

		ctx, cancel := context.WithCancel(context.Background())
		c.SetContext(ctx, cancel)
		teardownContainer(t, c, cancel)

		// ACT
		startHTTPSServer(ctx, c.Config(), c)

		// ASSERT
		require.NotNil(t, c.HTTPSServer().TLSConfig,
			"a successful cert source must install a TLS config on the HTTPS server")
	})
}

//nolint:paralleltest // the subtest stubs the shared osExit global
func TestRunWithGRPC(t *testing.T) {
	t.Run("calls_os_exit_when_the_grpc_bind_address_is_already_in_use", func(t *testing.T) {
		// ARRANGE
		port, release := takenPort(t)
		t.Cleanup(release)

		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.AuthService = authServicePaseto
			cfg.HTTPBindIP = loopbackBindIP
			cfg.HTTPPort = 0
			cfg.GRPC.Port = uint16(port)
		})
		ctx, cancel := context.WithCancel(context.Background())
		c.SetContext(ctx, cancel)
		teardownContainer(t, c, cancel)

		stubFatalExit(t)

		// ACT / ASSERT
		// runWithGRPC starts the session registry + dispatchers, builds the gRPC
		// server, then binds GRPC.Port. The bind fails on the occupied port and
		// the helper terminates via osExit before reaching the HTTP/HTTPS phase.
		assertFatalExit(t, func() {
			runWithGRPC(ctx, c.Config(), c)
		})
	})
}
