package daemon

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

const (
	statusRetryCount = 2
	statusRetryDelay = 10 * time.Millisecond
)

type StatusService struct {
	configMaker *configMaker

	mu    sync.RWMutex
	pools map[uint]*Pool
}

func NewStatusService(
	certRepo repositories.ClientCertificateRepository,
	fileManager files.FileManager,
) *StatusService {
	return &StatusService{
		configMaker: newConfigMaker(certRepo, fileManager),
		pools:       make(map[uint]*Pool),
	}
}

type NodeStatus struct {
	Uptime        time.Duration
	Version       string
	BuildDate     string
	WorkingTasks  int
	WaitingTasks  int
	OnlineServers int
}

type NodeVersion struct {
	Version   string
	BuildDate string
}

func (s *StatusService) Version(ctx context.Context, node *domain.Node) (*NodeVersion, error) {
	cfg, err := s.configMaker.Make(ctx, node)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var versionResp binnapi.StatusVersionResponseMessage

	err = Retry(statusRetryCount, statusRetryDelay, func() error {
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

		err = binnapi.WriteMessage(conn, binnapi.StatusRequestVersion)
		if err != nil {
			return errors.WithMessage(err, "failed to write version request")
		}

		err = binnapi.ReadMessage(conn, &versionResp)
		if err != nil {
			return errors.WithMessage(err, "failed to read version response")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to get version after %d attempts",
			statusRetryCount,
		)
	}

	return &NodeVersion{
		Version:   versionResp.Version,
		BuildDate: versionResp.BuildDate,
	}, nil
}

func (s *StatusService) Status(ctx context.Context, node *domain.Node) (*NodeStatus, error) {
	cfg, err := s.configMaker.Make(ctx, node)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to make config")
	}

	pool, err := s.getPool(node.ID, cfg)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get pool")
	}

	var versionResp binnapi.StatusVersionResponseMessage
	var baseResp binnapi.StatusInfoBaseResponseMessage

	err = Retry(statusRetryCount, statusRetryDelay, func() error {
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

		err = binnapi.WriteMessage(conn, binnapi.StatusRequestVersion)
		if err != nil {
			return errors.WithMessage(err, "failed to write version request")
		}

		err = binnapi.ReadMessage(conn, &versionResp)
		if err != nil {
			return errors.WithMessage(err, "failed to read version response")
		}

		err = binnapi.WriteMessage(conn, binnapi.StatusRequestStatusBase)
		if err != nil {
			return errors.WithMessage(err, "failed to write base status request")
		}

		err = binnapi.ReadMessage(conn, &baseResp)
		if err != nil {
			return errors.WithMessage(err, "failed to read base status response")
		}

		return nil
	})
	if err != nil {
		return nil, errors.WithMessagef(
			err,
			"failed to get status after %d attempts",
			statusRetryCount,
		)
	}

	uptime, err := time.ParseDuration(baseResp.Uptime)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse uptime")
	}

	var workingTasks int
	var waitingTasks int
	var onlineServers int

	if baseResp.WorkingTasks != "" && baseResp.WorkingTasks != "-" {
		workingTasks, err = strconv.Atoi(baseResp.WorkingTasks)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse working tasks")
		}
	}

	if baseResp.WaitingTasks != "" && baseResp.WaitingTasks != "-" {
		waitingTasks, err = strconv.Atoi(baseResp.WaitingTasks)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse waiting tasks")
		}
	}

	if baseResp.OnlineServers != "" && baseResp.OnlineServers != "-" {
		onlineServers, err = strconv.Atoi(baseResp.OnlineServers)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse online servers")
		}
	}

	return &NodeStatus{
		Uptime:        uptime,
		Version:       versionResp.Version,
		BuildDate:     versionResp.BuildDate,
		WorkingTasks:  workingTasks,
		WaitingTasks:  waitingTasks,
		OnlineServers: onlineServers,
	}, nil
}

func (s *StatusService) getPool(nodeID uint, cfg config) (*Pool, error) {
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
