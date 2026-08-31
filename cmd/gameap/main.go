package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gameap/gameap/internal/application"
	"github.com/gameap/gameap/internal/application/defaults"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	if isVersionCommand(os.Args[1:]) {
		printVersion(os.Stdout)

		return
	}

	envFile := flag.String("env", "", "Path to environment file")
	showVersion := flag.Bool("version", false, "Print version information and exit")

	flag.Parse()

	if *showVersion {
		printVersion(os.Stdout)

		return
	}

	slog.Info("Starting ...")

	application.Run(application.RunParams{
		EnvFile: *envFile,
	})
}

// isVersionCommand recognizes the bare `gameap version` form. The `-version`
// and `--version` flag forms are handled by the flag package itself.
func isVersionCommand(args []string) bool {
	return len(args) > 0 && args[0] == "version"
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "GameAP %s\n", defaults.Version)
	fmt.Fprintf(w, "Build date: %s\n", defaults.BuildDate)
}
