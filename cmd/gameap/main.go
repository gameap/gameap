package main

import (
	"flag"
	"log/slog"

	"github.com/gameap/gameap/internal/application"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	envFile := flag.String("env", "", "Path to environment file")

	flag.Parse()

	slog.Info("Starting ...")

	application.Run(application.RunParams{
		EnvFile: *envFile,
	})
}
