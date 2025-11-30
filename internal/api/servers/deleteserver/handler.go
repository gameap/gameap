package deleteserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type Handler struct {
	serverRepo     repositories.ServerRepository
	daemonTaskRepo repositories.DaemonTaskRepository
	rbac           base.RBAC
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	daemonTaskRepo repositories.DaemonTaskRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo:     serverRepo,
		daemonTaskRepo: daemonTaskRepo,
		rbac:           rbac,
		responder:      responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	serverID, err := api.NewInputReader(r).ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	var in input
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				errors.WithMessage(err, "invalid request body"),
				http.StatusBadRequest,
			))

			return
		}
	}

	servers, err := h.serverRepo.Find(ctx, filters.FindServerByIDs(serverID), nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find server"))

		return
	}

	if len(servers) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server not found"),
			http.StatusNotFound,
		))

		return
	}

	server := &servers[0]

	if in.DeleteFiles {
		if err := h.deleteWithFiles(ctx, server); err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete server with files"))

			return
		}
	} else {
		if err := h.deleteWithoutFiles(ctx, rw, server); err != nil {
			// Error already handled in deleteWithoutFiles

			return
		}
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteWithFiles(ctx context.Context, server *domain.Server) error {
	if err := h.createDeleteFileTasks(ctx, server); err != nil {
		return errors.WithMessage(err, "failed to create delete tasks")
	}

	if err := h.serverRepo.SoftDelete(ctx, server.ID); err != nil {
		return errors.WithMessage(err, "failed to soft delete server")
	}

	return nil
}

func (h *Handler) deleteWithoutFiles(ctx context.Context, rw http.ResponseWriter, server *domain.Server) error {
	if server.IsOnline() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("cannot delete server: server is online"),
			http.StatusConflict,
		))

		return errors.New("server is online")
	}

	serverID := server.ID
	hasPendingTasks, err := h.daemonTaskRepo.Exists(ctx, &filters.FindDaemonTask{
		ServerIDs: []*uint{&serverID},
		Statuses: []domain.DaemonTaskStatus{
			domain.DaemonTaskStatusWaiting,
			domain.DaemonTaskStatusWorking,
		},
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check pending tasks"))

		return err
	}

	if hasPendingTasks {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("cannot delete server: server has pending tasks"),
			http.StatusConflict,
		))

		return errors.New("server has pending tasks")
	}

	if err := h.serverRepo.Delete(ctx, server.ID); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete server"))

		return err
	}

	return nil
}

func (h *Handler) createDeleteFileTasks(ctx context.Context, server *domain.Server) error {
	var runAftID *uint

	if server.IsOnline() {
		stopTask := &domain.DaemonTask{
			DedicatedServerID: server.DSID,
			ServerID:          lo.ToPtr(server.ID),
			Task:              domain.DaemonTaskTypeServerStop,
			Status:            domain.DaemonTaskStatusWaiting,
			CreatedAt:         lo.ToPtr(time.Now()),
			UpdatedAt:         lo.ToPtr(time.Now()),
		}

		if err := h.daemonTaskRepo.Save(ctx, stopTask); err != nil {
			return errors.WithMessage(err, "failed to create stop task")
		}

		runAftID = lo.ToPtr(stopTask.ID)
	}

	deleteTask := &domain.DaemonTask{
		RunAftID:          runAftID,
		DedicatedServerID: server.DSID,
		ServerID:          lo.ToPtr(server.ID),
		Task:              domain.DaemonTaskTypeServerDelete,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         lo.ToPtr(time.Now()),
		UpdatedAt:         lo.ToPtr(time.Now()),
	}

	if err := h.daemonTaskRepo.Save(ctx, deleteTask); err != nil {
		return errors.WithMessage(err, "failed to create delete task")
	}

	return nil
}
