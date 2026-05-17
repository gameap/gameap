package console

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/shellescape"
)

const (
	legacyPollInterval = 500 * time.Millisecond
)

func (h *Handler) newLegacyMessageHandler(
	ctx context.Context,
	client *ws.Client,
	server *domain.Server,
	node *domain.Node,
	user *domain.User,
	canSend bool,
) ws.MessageHandler {
	return func(_ context.Context, msg *ws.InboundMessage) {
		if msg.Type != typeConsoleCommand {
			return
		}

		if !canSend {
			client.SendMessage(ws.NewErrorMessage("permission denied: cannot send commands"))

			return
		}

		var payload consoleCommandPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return
		}

		if payload.Command == "" {
			return
		}

		if err := h.abilityChecker.CheckOrError(
			ctx,
			user.ID,
			server.ID,
			[]domain.AbilityName{domain.AbilityNameGameServerConsoleSend},
		); err != nil {
			client.SendMessage(ws.NewErrorMessage("permission denied: cannot send commands"))

			return
		}

		h.sendLegacyCommand(ctx, client, server, node, payload.Command)
	}
}

func (h *Handler) sendLegacyCommand(
	ctx context.Context, client *ws.Client, server *domain.Server, node *domain.Node, command string,
) {
	if node.ScriptSendCommand != nil && *node.ScriptSendCommand != "" {
		cmd := server.ReplaceServerShortcodes(node, *node.ScriptSendCommand, map[string]string{
			"command": shellescape.Quote(command),
		})

		_, err := h.daemonCommands.ExecuteCommand(ctx, node, cmd)
		if err != nil {
			h.logger.Warn("failed to execute send command script",
				"server_id", server.ID,
				"error", err,
			)
			client.SendMessage(ws.NewErrorMessage("failed to send command"))
		}

		return
	}

	inputPath := filepath.Join(server.Dir, "input.txt")

	err := h.fileService.Upload(
		ctx, node, inputPath, []byte(command), 0o644, daemon.OwnerFromServer(server),
	)
	if err != nil {
		h.logger.Warn("failed to upload console command",
			"server_id", server.ID,
			"error", err,
		)
		client.SendMessage(ws.NewErrorMessage("failed to send command"))
	}
}

type legacyPoller struct {
	client      *ws.Client
	fileService fileService
	node        *domain.Node
	serverDir   string
	logger      *slog.Logger
	lastContent string
}

func newLegacyPoller(
	client *ws.Client,
	fileService fileService,
	node *domain.Node,
	serverDir string,
	logger *slog.Logger,
) *legacyPoller {
	return &legacyPoller{
		client:      client,
		fileService: fileService,
		node:        node,
		serverDir:   serverDir,
		logger:      logger,
	}
}

func (p *legacyPoller) run(ctx context.Context) {
	ticker := time.NewTicker(legacyPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.client.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *legacyPoller) poll(ctx context.Context) {
	outputPath := filepath.Join(p.serverDir, "output.txt")

	content, err := p.fileService.Download(ctx, p.node, outputPath)
	if err != nil {
		p.logger.Debug("legacy console poll: download failed",
			"path", outputPath,
			"error", err,
		)

		return
	}

	currentContent := string(content)
	if currentContent == p.lastContent {
		return
	}

	var diff string
	if len(currentContent) > len(p.lastContent) && currentContent[:len(p.lastContent)] == p.lastContent {
		diff = currentContent[len(p.lastContent):]
	} else {
		diff = currentContent
	}

	p.lastContent = currentContent

	if diff != "" {
		p.client.SendMessage(ws.NewOutboundMessage(typeConsoleOutput, consoleOutputPayload{
			Chunk: diff,
		}))
	}
}
