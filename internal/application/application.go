package application

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gameap/gameap/internal/application/defaults"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
	"github.com/gameap/gameap/internal/pubsub/integration"
	"github.com/gameap/gameap/migrations"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/netutil"
	"github.com/gameap/gameap/pkg/tlsutil"
	"github.com/pkg/errors"
)

type RunParams struct {
	EnvFile string
}

// osExit is a seam over os.Exit so the bootstrap failure paths can be exercised
// in tests without terminating the test process. Mirrors the detectInterfaceSANs
// seam in tls_helpers.go.
var osExit = os.Exit

//nolint:funlen
func Run(runParams RunParams) {
	if err := loadEnvFile(runParams.EnvFile); err != nil {
		slog.Error("Failed to load env file", slog.String("error", err.Error()))

		osExit(1)

		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load config", slog.String("error", err.Error()))

		osExit(1)

		return
	}

	logLevel := slog.LevelInfo

	switch cfg.Logger.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "error":
		logLevel = slog.LevelError
	}

	slog.SetLogLoggerLevel(logLevel)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())

	if cfg.HTTPBindIP == "" {
		cfg.HTTPBindIP = netutil.ResolveBindAddress(ctx, cfg.HTTPHost)
	}

	container := NewContainer(cfg)
	container.SetContext(ctx, cancel)

	// Install the process-wide password policy state used by
	// auth.ValidatePassword (ASVS §2.1.7 common-password blocklist + the
	// AUTH_ALLOW_WEAK_PASSWORDS operator override). The Container accessor
	// emits the warn/info/error logs; we wire the resulting blocklist into
	// the pkg/auth singleton so every handler input.Validate() sees it.
	auth.SetAllowWeakPasswords(cfg.Auth.AllowWeakPasswords)
	auth.SetPasswordBlocklist(container.PasswordBlocklist())

	// Install the operator-chosen bcrypt cost (ASVS §2.4.4). A configured
	// value outside [auth.MinBcryptCost, auth.MaxBcryptCost] is rejected at
	// boot rather than silently falling back, so a misconfiguration cannot
	// ship a weaker default than the project floor.
	if err := auth.SetDefaultBcryptCost(cfg.Auth.BcryptCost); err != nil {
		slog.Error("invalid AUTH_BCRYPT_COST", slog.Int("cost", cfg.Auth.BcryptCost), slog.String("error", err.Error()))
		osExit(1)

		return
	}
	slog.Info("password hashing configured",
		slog.Int("bcrypt_cost", auth.ActiveBcryptCost()))

	// The node path policy is a security setting: a typo must not silently
	// fall back to "unrestricted".
	if _, err := hostlibrary.NewPathPolicy(hostlibrary.PathPolicyConfig{
		Mode:         hostlibrary.PathPolicyMode(cfg.Plugin.NodeFS.PathPolicy),
		AllowedPaths: cfg.Plugin.NodeFS.AllowedPaths,
	}, container.ServerRepository()); err != nil {
		slog.Error("invalid PLUGIN_NODEFS_PATH_POLICY / PLUGIN_NODEFS_ALLOWED_PATHS", slog.String("error", err.Error()))
		osExit(1)

		return
	}

	slog.Info("plugin node path policy configured",
		slog.String("mode", cfg.Plugin.NodeFS.PathPolicy),
		slog.Int("allowed_paths", len(cfg.Plugin.NodeFS.AllowedPaths)))

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		oscall := <-sigCh

		slog.Info("Got signal: " + oscall.String())

		if err := container.Shutdown(); err != nil {
			slog.ErrorContext(
				ctx,
				"Failed to shutdown container",
				slog.String("error", err.Error()),
			)
		}
	}()

	err = migrations.Run(ctx, container)
	if err != nil {
		slog.ErrorContext(
			ctx,
			"Failed to run migrations",
			slog.String("error", err.Error()),
		)

		osExit(1)

		return
	}

	err = seed(ctx, container)
	if err != nil {
		slog.ErrorContext(
			ctx,
			"Failed to seed database",
			slog.String("error", err.Error()),
		)

		osExit(1)

		return
	}

	slog.InfoContext(
		ctx,
		"GameAP started",
		slog.String("version", defaults.Version),
		slog.String("build_date", defaults.BuildDate),
	)

	if cfg.Plugins.Disabled {
		slog.InfoContext(ctx, "Plugins are disabled, skipping plugin loading")
	} else {
		startPluginServices(ctx, container)

		err = container.PluginDispatcher().RefreshSubscriptions(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to refresh plugin subscriptions", slog.String("error", err.Error()))

			osExit(1)
		}

		startPluginSync(ctx, container)
	}

	startPubSub(ctx, container)

	startUploadJanitor(ctx, container)

	runWithGRPC(ctx, cfg, container)

	<-shutdownDone
}

// startPluginServices brings up the services plugins register into and then
// loads the plugins. The scheduler and the archive event dispatcher start
// before LoadAll so registrations made during guest Initialize land on
// running services.
func startPluginServices(ctx context.Context, container *Container) {
	if err := container.PluginScheduler().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start plugin scheduler", slog.String("error", err.Error()))

		osExit(1)

		return
	}

	if err := container.PluginArchiveEvents().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start plugin archive events", slog.String("error", err.Error()))

		osExit(1)

		return
	}

	if container.config.Plugin.SSH.Enabled {
		// Records the lifetime context before any guest Initialize can open a
		// connection, so completion callbacks outlive the calls that start them.
		container.PluginSSH().Start(ctx)
	}

	// Subscribed before LoadAll so a peer's change during the load window
	// is not missed; the first pass runs once the subscriptions are built.
	if err := container.PluginSync().Subscribe(ctx); err != nil {
		slog.WarnContext(ctx, "Failed to subscribe to plugin sync hints", slog.String("error", err.Error()))
	}

	if err := container.PluginLoader().LoadAll(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to load plugins", slog.String("error", err.Error()))

		osExit(1)
	}
}

// startPluginSync runs the first reconcile pass (synchronously, so the
// startup state is recorded before hints arrive) and keeps reconciling in
// the background. A nil service (sync disabled) is a no-op.
func startPluginSync(ctx context.Context, container *Container) {
	if err := container.PluginSync().Start(ctx); err != nil {
		slog.WarnContext(ctx, "Failed to start plugin sync", slog.String("error", err.Error()))
	}
}

// startHTTPListener binds the plain-HTTP listener and starts serving on it
// in a background goroutine. We deliberately bind synchronously (so that a
// "port already in use" failure is visible at startup) and serve async — the
// listener must already be accepting connections before startHTTPSServer
// kicks off the ACME HTTP-01 challenge, otherwise Let's Encrypt will hit a
// connection-refused on /.well-known/acme-challenge/.
func startHTTPListener(ctx context.Context, cfg *config.Config, container *Container) {
	server := container.HTTPServer()

	addr := net.JoinHostPort(cfg.HTTPBindIP, strconv.Itoa(int(cfg.HTTPPort)))

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to bind HTTP listener",
			slog.String("address", addr),
			slog.String("error", err.Error()),
		)

		osExit(1)
	}

	go func() {
		slog.InfoContext(ctx, "Starting HTTP server", slog.String("address", addr))

		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "HTTP server error", slog.String("error", err.Error()))
		}
	}()
}

func runWithGRPC(ctx context.Context, cfg *config.Config, container *Container) {
	if err := container.SessionRegistry().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start session registry", slog.String("error", err.Error()))
		osExit(1)
	}

	if err := container.FileDispatcher().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start file dispatcher", slog.String("error", err.Error()))
	}

	if err := container.DaemonArchive().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start archive service", slog.String("error", err.Error()))
	}

	if err := container.CommandDispatcher().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start command dispatcher", slog.String("error", err.Error()))
	}

	if err := container.StatusDispatcher().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start status dispatcher", slog.String("error", err.Error()))
	}

	if err := container.ConsoleLogDispatcher().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start console log dispatcher", slog.String("error", err.Error()))
	}

	if err := container.HTTPProxyDispatcher().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start HTTP proxy dispatcher", slog.String("error", err.Error()))
	}

	if err := container.MetricsHub().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start metrics hub", slog.String("error", err.Error()))
	}

	if err := container.TaskReaper().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start task reaper", slog.String("error", err.Error()))
	}

	grpcServer, err := container.GRPCServer()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create gRPC server", slog.String("error", err.Error()))
		osExit(1)
	}

	grpcAddr := net.JoinHostPort(cfg.HTTPBindIP, strconv.Itoa(int(cfg.GRPC.Port)))

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", grpcAddr)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to listen for gRPC", slog.String("error", err.Error()))
		osExit(1)
	}

	go func() {
		slog.InfoContext(ctx, "Starting gRPC server",
			slog.String("address", grpcAddr),
			slog.Bool("tls_enabled", cfg.GRPC.TLSEnabled),
			slog.Bool("mtls_required", cfg.GRPC.RequireMTLS),
			slog.Bool("reflection_enabled", cfg.GRPC.EnableReflection),
			slog.Int("max_recv_mb", cfg.GRPC.MaxRecvMsgSize/(1<<20)),
			slog.Int("max_send_mb", cfg.GRPC.MaxSendMsgSize/(1<<20)),
		)
		if err := grpcServer.Serve(lis); err != nil {
			slog.ErrorContext(ctx, "gRPC server error", slog.String("error", err.Error()))
		}
	}()

	startHTTPListener(ctx, cfg, container)

	if cfg.TLSEnabled() {
		startHTTPSServer(ctx, cfg, container)
	}
}

func startHTTPSServer(ctx context.Context, cfg *config.Config, container *Container) {
	getCert, err := buildGetCertificate(ctx, cfg, container)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialise TLS certificate source",
			slog.String("source", cfg.EffectiveCertSource().String()),
			slog.String("error", err.Error()),
		)

		osExit(1)

		return
	}

	httpsServer := container.HTTPSServer()
	httpsServer.TLSConfig = tlsutil.HardenServerConfig(&tls.Config{
		GetCertificate: getCert,
	})

	go func() {
		slog.InfoContext(ctx, "Starting HTTPS server",
			slog.String("address", net.JoinHostPort(cfg.HTTPBindIP, strconv.Itoa(int(cfg.HTTPSPort)))),
			slog.String("cert_source", cfg.EffectiveCertSource().String()),
		)

		err := httpsServer.ListenAndServeTLS("", "")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "HTTPS server error", slog.String("error", err.Error()))
		}
	}()
}

func buildGetCertificate(
	ctx context.Context,
	cfg *config.Config,
	container *Container,
) (func(*tls.ClientHelloInfo) (*tls.Certificate, error), error) {
	source := cfg.EffectiveCertSource()

	switch source {
	case config.CertSourceACME:
		if cfg.TLS.CertFile != "" || cfg.TLS.Cert != "" {
			slog.WarnContext(ctx, "ACME is enabled; static TLS_CERT_FILE/TLS_CERT settings are ignored")
		}

		acmeService := container.ACMEService()
		if err := acmeService.Start(ctx); err != nil {
			return nil, errors.WithMessage(err, "ACME initialisation failed")
		}

		return acmeService.GetCertificate, nil
	case config.CertSourceFile, config.CertSourceInline:
		cert, err := cfg.LoadTLSCertificate()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load static TLS certificate")
		}

		return func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return cert, nil
		}, nil
	case config.CertSourceNone:
		return nil, errors.New("no certificate source configured but TLS startup attempted")
	default:
		return nil, errors.Errorf("unsupported cert source %q", source)
	}
}

func startUploadJanitor(ctx context.Context, container *Container) {
	janitor := container.UploadJanitor()
	go func() {
		slog.InfoContext(ctx, "Starting upload session janitor",
			slog.Duration("interval", container.Config().Files.Upload.JanitorInterval),
			slog.Duration("session_ttl", container.Config().Files.Upload.SessionTTL),
		)
		janitor.Run(ctx)
	}()
}

func startPubSub(ctx context.Context, container *Container) {
	ps := container.PubSub()

	cacheInvalidator := integration.NewCacheInvalidator(ps, container.Cache())
	if err := cacheInvalidator.Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start cache invalidator", slog.String("error", err.Error()))
	}

	if err := container.PluginSubscriptionsNotifier().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start plugin subscriptions notifier",
			slog.String("error", err.Error()))
	}

	if err := container.WSBridge().Start(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to start WebSocket bridge", slog.String("error", err.Error()))
	}

	go func() {
		slog.InfoContext(ctx, "Starting pub-sub listener",
			slog.String("driver", container.Config().PubSub.Driver),
		)

		if err := ps.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.ErrorContext(ctx, "Pub-sub listener error", slog.String("error", err.Error()))
		}
	}()
}
