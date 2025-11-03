package application

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/samber/lo"
)

func loadLegacyEnv(legacyEnvFilePath string) error {
	legacyEnvPath := lo.CoalesceOrEmpty(os.Getenv("LEGACY_ENV_PATH"), legacyEnvFilePath)
	if legacyEnvPath == "" {
		return nil
	}

	if _, err := os.Stat(legacyEnvPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	legacyVars, err := parseLegacyEnvFile(legacyEnvPath)
	if err != nil {
		return err
	}

	convertLegacyEnvToCurrentEnv(legacyVars)

	return nil
}

func parseLegacyEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			slog.Warn(
				"Failed to close file",
				slog.String("path", path),
				slog.String("error", err.Error()),
			)
		}
	}(file)

	envVars := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		envVars[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return envVars, nil
}

func convertLegacyEnvToCurrentEnv(legacyVars map[string]string) {
	if dbConnection, ok := legacyVars["DB_CONNECTION"]; ok && dbConnection != "" {
		setEnvIfNotExists("DATABASE_DRIVER", dbConnection)
	}

	buildDatabaseURL(legacyVars)

	if appKey, ok := legacyVars["APP_KEY"]; ok && appKey != "" {
		setEnvIfNotExists("ENCRYPTION_KEY", appKey)
		setEnvIfNotExists("AUTH_SECRET", appKey)
	}

	if appDebug, ok := legacyVars["APP_DEBUG"]; ok {
		logLevel := "info"
		if appDebug == "true" {
			logLevel = "debug"
		}
		setEnvIfNotExists("LOGGER_LEVEL", logLevel)
	}

	if cacheDriver, ok := legacyVars["CACHE_DRIVER"]; ok && cacheDriver != "" {
		if cacheDriver == "file" {
			cacheDriver = "memory"
		}
		setEnvIfNotExists("CACHE_DRIVER", cacheDriver)
	}

	buildRedisAddr(legacyVars)

	if redisPassword, ok := legacyVars["REDIS_PASSWORD"]; ok && redisPassword != "" && redisPassword != "null" {
		setEnvIfNotExists("CACHE_REDIS_PASSWORD", redisPassword)
	}
}

func buildDatabaseURL(legacyVars map[string]string) {
	if os.Getenv("DATABASE_URL") != "" {
		return
	}

	dbHost := legacyVars["DB_HOST"]
	dbPort := legacyVars["DB_PORT"]
	dbDatabase := legacyVars["DB_DATABASE"]
	dbUsername := legacyVars["DB_USERNAME"]
	dbPassword := legacyVars["DB_PASSWORD"]
	dbConnection := legacyVars["DB_CONNECTION"]

	if dbConnection == "" {
		dbConnection = "mysql"
	}

	if dbHost == "" || dbDatabase == "" || dbUsername == "" {
		return
	}

	if dbPort == "" {
		dbPort = "3306"
	}

	var databaseURL string
	if dbConnection == "mysql" {
		databaseURL = fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?parseTime=true",
			dbUsername, dbPassword, dbHost, dbPort, dbDatabase,
		)
	}

	if databaseURL != "" {
		_ = os.Setenv("DATABASE_URL", databaseURL)
	}
}

func buildRedisAddr(legacyVars map[string]string) {
	if os.Getenv("CACHE_REDIS_ADDR") != "" {
		return
	}

	redisHost := legacyVars["REDIS_HOST"]
	redisPort := legacyVars["REDIS_PORT"]

	if redisHost == "" {
		return
	}

	if redisPort == "" {
		redisPort = "6379"
	}

	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)
	_ = os.Setenv("CACHE_REDIS_ADDR", redisAddr)
}

func setEnvIfNotExists(key, value string) {
	if os.Getenv(key) == "" && value != "" {
		_ = os.Setenv(key, value)
	}
}
