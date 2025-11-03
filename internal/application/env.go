package application

import (
	"bufio"
	"log/slog"
	"os"
	"strings"

	"github.com/pkg/errors"
)

func loadEnvFile(filePath string) error {
	if filePath == "" {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to open env file")
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			slog.Warn("Failed to close env file")
		}
	}(file)

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return errors.Errorf("invalid env file format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		value = strings.Trim(value, "\"'")

		if err := os.Setenv(key, value); err != nil {
			return errors.Wrapf(err, "failed to set environment variable %s", key)
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "failed to read env file")
	}

	return nil
}
