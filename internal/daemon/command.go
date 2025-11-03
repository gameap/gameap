package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

const (
	commandsRetryCount = 2
	commandsRetryDelay = 10 * time.Millisecond
)

type CommandService struct {
	configMaker *configMaker

	mu    sync.RWMutex
	pools map[uint]*Pool
}

func NewCommandService(
	certRepo repositories.ClientCertificateRepository,
	fileManager files.FileManager,
) *CommandService {
	return &CommandService{
		configMaker: newConfigMaker(certRepo, fileManager),
		pools:       make(map[uint]*Pool),
	}
}

type CommandResult struct {
	Output   string
	ExitCode int
}

func (s *CommandService) ExecuteCommand(
	ctx context.Context,
	node *domain.Node,
	command string,
	opts ...CommandServiceOption,
) (*CommandResult, error) {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeCMD)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	// Send command execution request
	req := binnapi.CommandExecRequestMessage{
		Command: command,
		WorkDir: "/",
	}

	// Apply options
	for _, opt := range opts {
		opt(&req)
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var resp binnapi.CommandExecResponseMessage

	err = Retry(commandsRetryCount, commandsRetryDelay, func() error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to acquire connection from pool")
		}
		defer func() {
			err = conn.Close()
			if err != nil {
				slog.Warn("failed to close connection", "error", err)
			}
		}()

		err = binnapi.WriteMessage(conn, req)
		if err != nil {
			return errors.WithMessage(err, "failed to write command request")
		}

		msg, err := binnapi.ReadMessageToSlice(ctx, conn)
		if err != nil {
			return errors.WithMessage(err, "failed to read command response to slice")
		}

		err = resp.FillFromSlice(msg)
		if err != nil {
			var baseResp binnapi.BaseResponseMessage
			baseRespErr := baseResp.FillFromSlice(msg)
			if baseRespErr != nil {
				return errors.WithMessage(err, "failed to parse command response")
			}

			resp.Code = baseResp.Code
			resp.Output = baseResp.Info
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to execute command after %d attempts",
			commandsRetryCount,
		)
	}

	return &CommandResult{
		Output:   resp.Output,
		ExitCode: resp.ExitCode,
	}, nil
}

func (s *CommandService) getPool(nodeID uint, cfg config) (*Pool, error) {
	s.mu.RLock()
	pool, exists := s.pools[nodeID]
	s.mu.RUnlock()

	if exists {
		return pool, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check existence to avoid race condition
	pool, exists = s.pools[nodeID]
	if exists {
		return pool, nil
	}

	pool, err := NewPool(cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create pool")
	}

	s.pools[nodeID] = pool

	return pool, nil
}

type CommandServiceOption func(*binnapi.CommandExecRequestMessage)

func CommandServiceOptionWithWorkDir(workDir string) CommandServiceOption {
	return func(msg *binnapi.CommandExecRequestMessage) {
		msg.WorkDir = workDir
	}
}
