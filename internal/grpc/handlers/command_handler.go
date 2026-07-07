package handlers

import (
	"context"
	"log/slog"
	"sync"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
)

type CommandResult struct {
	CommandID string
	ExitCode  int32
	Output    []byte
	Error     string
}

type CommandHandler struct {
	publisher pubsub.Publisher
	logger    *slog.Logger

	mu              sync.RWMutex
	pendingCommands map[string]chan *CommandResult
	commandServers  map[string]uint64 // command_id → server_id
}

func NewCommandHandler(publisher pubsub.Publisher, logger *slog.Logger) *CommandHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &CommandHandler{
		publisher:       publisher,
		logger:          logger,
		pendingCommands: make(map[string]chan *CommandResult),
		commandServers:  make(map[string]uint64),
	}
}

func (h *CommandHandler) HandleCommandOutput(ctx context.Context, nodeID uint64, output *proto.CommandOutput) error {
	h.logger.Debug("received command output",
		"node_id", nodeID,
		"command_id", output.CommandId,
		"bytes", len(output.OutputChunk),
	)

	h.mu.RLock()
	serverID, hasServer := h.commandServers[output.CommandId]
	h.mu.RUnlock()

	if hasServer && h.publisher != nil {
		h.publishConsoleOutput(ctx, serverID, output.CommandId, string(output.OutputChunk))
	}

	return nil
}

func (h *CommandHandler) HandleCommandResult(ctx context.Context, nodeID uint64, result *proto.CommandResult) error {
	h.logger.Debug("received command result",
		"node_id", nodeID,
		"command_id", result.CommandId,
		"exit_code", result.ExitCode,
		"error", result.Error,
	)

	// Delete-under-lock to take exclusive ownership of the channel before
	// sending/closing, so a concurrent UnregisterPendingCommand (which also
	// deletes-then-closes under the lock) can never close a channel this send
	// is still using — that would panic. Mirrors session.ResolvePendingRequest.
	h.mu.Lock()
	ch, hasPending := h.pendingCommands[result.CommandId]
	if hasPending {
		delete(h.pendingCommands, result.CommandId)
	}
	serverID, hasServer := h.commandServers[result.CommandId]
	h.mu.Unlock()

	if hasPending {
		select {
		case ch <- &CommandResult{
			CommandID: result.CommandId,
			ExitCode:  result.ExitCode,
			Output:    result.Output,
			Error:     result.Error,
		}:
		default:
		}
		close(ch)
	}

	if hasServer && h.publisher != nil {
		h.publishConsoleResult(ctx, serverID, result.CommandId, result.ExitCode, result.Error)
	}

	h.UntrackCommandServer(result.CommandId)

	return nil
}

func (h *CommandHandler) RegisterPendingCommand(commandID string) <-chan *CommandResult {
	ch := make(chan *CommandResult, 1)
	h.mu.Lock()
	h.pendingCommands[commandID] = ch
	h.mu.Unlock()

	return ch
}

func (h *CommandHandler) UnregisterPendingCommand(commandID string) {
	h.mu.Lock()
	if ch, ok := h.pendingCommands[commandID]; ok {
		close(ch)
		delete(h.pendingCommands, commandID)
	}
	delete(h.commandServers, commandID)
	h.mu.Unlock()
}

func (h *CommandHandler) TrackCommandServer(commandID string, serverID uint64) {
	h.mu.Lock()
	h.commandServers[commandID] = serverID
	h.mu.Unlock()
}

func (h *CommandHandler) UntrackCommandServer(commandID string) {
	h.mu.Lock()
	delete(h.commandServers, commandID)
	h.mu.Unlock()
}

func (h *CommandHandler) publishConsoleOutput(ctx context.Context, serverID uint64, commandID string, chunk string) {
	channel := channels.BuildRealtimeConsoleOutputChannel(serverID)

	msg, err := messages.NewMessage(channel, messages.TypeConsoleOutput, messages.ConsoleOutputPayload{
		ServerID:  serverID,
		CommandID: commandID,
		Chunk:     chunk,
	})
	if err != nil {
		h.logger.Warn("failed to create console output message", "error", err)

		return
	}

	if err := h.publisher.Publish(ctx, channel, msg); err != nil {
		h.logger.Warn("failed to publish console output", "error", err)
	}
}

func (h *CommandHandler) publishConsoleResult(
	ctx context.Context, serverID uint64, commandID string, exitCode int32, resultErr string,
) {
	channel := channels.BuildRealtimeConsoleResultChannel(serverID)

	msg, err := messages.NewMessage(channel, messages.TypeConsoleResult, messages.ConsoleResultPayload{
		ServerID:  serverID,
		CommandID: commandID,
		ExitCode:  exitCode,
		Error:     resultErr,
	})
	if err != nil {
		h.logger.Warn("failed to create console result message", "error", err)

		return
	}

	if err := h.publisher.Publish(ctx, channel, msg); err != nil {
		h.logger.Warn("failed to publish console result", "error", err)
	}
}
