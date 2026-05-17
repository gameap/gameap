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

type CommandBINNService struct {
	configMaker *configMaker

	mu    sync.RWMutex
	pools map[uint]*Pool
}

func NewCommandBINNService(
	certRepo repositories.ClientCertificateRepository,
	fileManager files.FileManager,
	opts ...LegacyOption,
) *CommandBINNService {
	cm := newConfigMaker(certRepo, fileManager)
	for _, opt := range opts {
		opt(cm)
	}

	return &CommandBINNService{
		configMaker: cm,
		pools:       make(map[uint]*Pool),
	}
}

func (s *CommandBINNService) ExecuteCommand(
	ctx context.Context,
	node *domain.Node,
	command string,
	opts ...CommandServiceOption,
) (*CommandResult, error) {
	cfg, err := s.configMaker.MakeWithMode(ctx, node, binnapi.ModeCMD)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	o := applyCommandOptions(opts)

	req := binnapi.CommandExecRequestMessage{
		Command: command,
		WorkDir: o.WorkDir,
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

func (s *CommandBINNService) getPool(nodeID uint, cfg config) (*Pool, error) {
	s.mu.RLock()
	pool, exists := s.pools[nodeID]
	s.mu.RUnlock()

	if exists {
		return pool, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

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
