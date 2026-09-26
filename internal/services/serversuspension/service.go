package serversuspension

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/pkg/errors"
)

// Service suspends servers and lifts suspensions. A suspension is the flag
// with its record, pushed to the daemon at once so that it refuses starts,
// and a stop of the server. Refusing what would run the server again is
// servercontrol's part.
type Service struct {
	serverRepo   repositories.ServerRepository
	taskRepo     repositories.DaemonTaskRepository
	stopper      serverStopper
	configPusher configPusher
	audit        audit.Logger
	logger       *slog.Logger
	now          func() time.Time
}

func NewService(
	serverRepo repositories.ServerRepository,
	taskRepo repositories.DaemonTaskRepository,
	stopper serverStopper,
	configPusher configPusher,
	auditLogger audit.Logger,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{
		serverRepo:   serverRepo,
		taskRepo:     taskRepo,
		stopper:      stopper,
		configPusher: configPusher,
		audit:        auditLogger,
		logger:       logger,
		now:          time.Now,
	}
}

// Suspend suspends the server and stops it. It returns the ID of the stop task
// to wait for, 0 when none was queued. Suspending a suspended server keeps its
// date, replaces the reason when one is given, and stops the server again only
// when it is reported running.
func (s *Service) Suspend(ctx context.Context, server *domain.Server, reason *string) (uint, error) {
	wasSuspended := server.IsSuspended()

	server.Suspend(s.now(), reason)

	if err := s.serverRepo.Save(ctx, server); err != nil {
		return 0, errors.WithMessage(err, "failed to save server")
	}

	s.pushConfig(ctx, server)

	stopTaskID := s.stop(ctx, server, wasSuspended)

	s.auditSuspend(ctx, server, stopTaskID)

	return stopTaskID, nil
}

// Unsuspend lifts the suspension. Nothing is started: running the server again
// is a decision of its own. A server that is not suspended is left as it is.
func (s *Service) Unsuspend(ctx context.Context, server *domain.Server) error {
	if !server.IsSuspended() {
		return nil
	}

	server.Unsuspend()

	if err := s.serverRepo.Save(ctx, server); err != nil {
		return errors.WithMessage(err, "failed to save server")
	}

	s.pushConfig(ctx, server)

	s.auditUnsuspend(ctx, server)

	return nil
}

// AfterUpdate completes a suspension, or its lifting, that an update of the
// whole server has already saved and pushed: it stops a newly suspended server
// and records the change. It returns the ID of the stop task, 0 when none was
// queued.
func (s *Service) AfterUpdate(ctx context.Context, server *domain.Server, wasSuspended bool) uint {
	switch {
	case server.IsSuspended() && !wasSuspended:
		stopTaskID := s.stop(ctx, server, false)
		s.auditSuspend(ctx, server, stopTaskID)

		return stopTaskID
	case !server.IsSuspended() && wasSuspended:
		s.auditUnsuspend(ctx, server)
	}

	return 0
}

// stop queues the stop of a suspended server, reusing one already queued or
// running. A fresh suspension stops whatever may be running; a repeated one
// only a server reported running, so a billing system that re-applies its
// suspensions does not queue a stop every time. A stop that cannot be queued
// leaves the suspension in force: the daemon stops a suspended server itself.
func (s *Service) stop(ctx context.Context, server *domain.Server, wasSuspended bool) uint {
	if taskID := s.inFlightStop(ctx, server); taskID != 0 {
		return taskID
	}

	needed := server.MayBeRunning() || s.startedSinceReport(ctx, server)
	if wasSuspended {
		needed = server.IsOnline()
	}

	if !needed {
		return 0
	}

	taskID, err := s.stopper.Stop(ctx, server)
	if err == nil {
		return taskID
	}

	var alreadyExists *servercontrol.TaskAlreadyExistsError
	if errors.As(err, &alreadyExists) {
		return s.inFlightStop(ctx, server)
	}

	s.logger.WarnContext(ctx, "failed to stop suspended server",
		"server_id", server.ID,
		"error", err,
	)

	return 0
}

// startedSinceReport reports whether the last start or restart of the server
// is queued, running or finished after the daemon last reported the server
// down: that report does not account for it. The daemon reports every few
// seconds, so this covers a suspension that follows a start at once. When the
// tasks cannot be read, stopping is the safe side.
func (s *Service) startedSinceReport(ctx context.Context, server *domain.Server) bool {
	if server.Installed != domain.ServerInstalledStatusInstalled || server.LastProcessCheck == nil {
		return false
	}

	tasks, err := s.taskRepo.Find(ctx, &filters.FindDaemonTask{
		ServerIDs: []*uint{&server.ID},
		Tasks: []domain.DaemonTaskType{
			domain.DaemonTaskTypeServerStart,
			domain.DaemonTaskTypeServerRestart,
		},
	}, []filters.Sorting{{Field: "id", Direction: filters.SortDirectionDesc}}, &filters.Pagination{Limit: 1})
	if err != nil {
		s.logger.WarnContext(ctx, "failed to look up the last start of suspended server",
			"server_id", server.ID,
			"error", err,
		)

		return true
	}

	if len(tasks) == 0 {
		return false
	}

	last := tasks[0]

	switch last.Status {
	case domain.DaemonTaskStatusWaiting, domain.DaemonTaskStatusWorking:
		return true
	case domain.DaemonTaskStatusSuccess:
		return last.UpdatedAt != nil && !last.UpdatedAt.Before(*server.LastProcessCheck)
	default:
		return false
	}
}

// inFlightStop returns the ID of a stop of the server that is queued or
// running, 0 when there is none.
func (s *Service) inFlightStop(ctx context.Context, server *domain.Server) uint {
	tasks, err := s.taskRepo.Find(ctx, &filters.FindDaemonTask{
		ServerIDs: []*uint{&server.ID},
		Tasks:     []domain.DaemonTaskType{domain.DaemonTaskTypeServerStop},
		Statuses: []domain.DaemonTaskStatus{
			domain.DaemonTaskStatusWaiting,
			domain.DaemonTaskStatusWorking,
		},
	}, nil, nil)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to look up the stop of suspended server",
			"server_id", server.ID,
			"error", err,
		)

		return 0
	}

	var taskID uint
	for i := range tasks {
		taskID = max(taskID, tasks[i].ID)
	}

	return taskID
}

func (s *Service) pushConfig(ctx context.Context, server *domain.Server) {
	if s.configPusher == nil {
		return
	}

	s.configPusher.PushServerConfig(ctx, server.ID)
}

// The reason stays out of the audit record: it is free text a billing system
// may fill with customer details.
func (s *Service) auditSuspend(ctx context.Context, server *domain.Server, stopTaskID uint) {
	audit.SensitiveOp(ctx, s.audit, audit.EventServerSuspend, audit.CategoryAdminOp,
		"server", strconv.FormatUint(uint64(server.ID), 10), "suspend",
		slog.Uint64("node_id", uint64(server.DSID)),
		slog.Uint64("stop_task_id", uint64(stopTaskID)),
	)
}

func (s *Service) auditUnsuspend(ctx context.Context, server *domain.Server) {
	audit.SensitiveOp(ctx, s.audit, audit.EventServerUnsuspend, audit.CategoryAdminOp,
		"server", strconv.FormatUint(uint64(server.ID), 10), "unsuspend",
		slog.Uint64("node_id", uint64(server.DSID)),
	)
}
