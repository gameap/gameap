//go:build wasip1

package main

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
)

// statsReportInterval is 5 minutes; runs are aligned to Unix-epoch multiples
// of the interval by the panel scheduler.
const statsReportIntervalMs = 300_000

type scheduledTaskHandler struct{}

func (h *scheduledTaskHandler) HandleScheduledTask(
	_ context.Context,
	req *scheduler.HandleScheduledTaskRequest,
) (*scheduler.HandleScheduledTaskResponse, error) {
	logger.Info("scheduled task fired",
		slog.String("task", req.TaskName),
		slog.Int64("scheduled_at", req.ScheduledAt),
		slog.Uint64("attempt", uint64(req.Attempt)),
		slog.Uint64("events_seen", eventCounter.Load()),
	)

	return &scheduler.HandleScheduledTaskResponse{}, nil
}

// registerStatsReportTask upserts the demo task; re-registration on every
// plugin load is safe.
func registerStatsReportTask(ctx context.Context) {
	resp, err := schedulerSvc.AddTask(ctx, &scheduler.AddTaskRequest{
		Name:       "stats-report",
		IntervalMs: statsReportIntervalMs,
		ErrorPolicy: &scheduler.ErrorPolicy{
			Policy:       scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_RETRY,
			MaxRetries:   3,
			RetryDelayMs: 1_000,
			MaxJitterMs:  500,
		},
		TimeoutMs: 5_000,
	})
	if err != nil {
		logger.Warn("failed to register scheduled task", slog.String("error", err.Error()))

		return
	}

	if !resp.Success {
		errMsg := ""
		if resp.Error != nil {
			errMsg = *resp.Error
		}
		logger.Warn("scheduled task registration rejected", slog.String("error", errMsg))
	}
}
