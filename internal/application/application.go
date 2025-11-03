package application

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/gameap/gameap/internal/application/defaults"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/pkg/errors"
)

type RunParams struct {
	EnvFile       string
	LegacyEnvFile string
}

//nolint:funlen
func Run(runParams RunParams) {
	if err := loadEnvFile(runParams.EnvFile); err != nil {
		slog.Error("Failed to load env file", slog.String("error", err.Error()))

		os.Exit(1)

		return
	}

	if err := loadLegacyEnv(runParams.LegacyEnvFile); err != nil {
		slog.Error("Failed to load legacy env file", slog.String("error", err.Error()))

		os.Exit(1)

		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error(err.Error())

		os.Exit(1)

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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())

	container := NewContainer(cfg)
	container.SetContext(ctx)

	go func() {
		oscall := <-c

		slog.Info("Got signal: " + oscall.String())

		cancel()

		err = container.Shutdown()
		if err != nil {
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

		os.Exit(1)

		return
	}

	err = seed(ctx, container)
	if err != nil {
		slog.ErrorContext(
			ctx,
			"Failed to seed database",
			slog.String("error", err.Error()),
		)

		os.Exit(1)

		return
	}

	slog.InfoContext(
		ctx,
		"GameAP started",
		slog.String("version", defaults.Version),
		slog.String("build_date", defaults.BuildDate),
	)

	slog.InfoContext(ctx, fmt.Sprintf("Starting server on %s:%d", cfg.HTTPHost, cfg.HTTPPort))

	server := container.HTTPServer()

	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error(err.Error())

		os.Exit(1)

		return
	}
}
