package paste

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	operationTypeCopy = "copy"
	operationTypeCut  = "cut"
)

type fileService interface {
	Copy(ctx context.Context, node *domain.Node, source, destination string) error
	Move(ctx context.Context, node *domain.Node, source, destination string) error
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	daemonFiles    fileService
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonFiles fileService,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		daemonFiles:    daemonFiles,
		responder:      responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	input := api.NewInputReader(r)

	serverID, err := input.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.abilityChecker.CheckOrError(
		ctx,
		session.User.ID,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerFiles},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	var req pasteRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	if err = h.validateRequest(&req); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err = h.processItems(ctx, node, server.Dir, &req); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newPasteResponse(req.Clipboard.Type))
}

func (h *Handler) validateRequest(req *pasteRequest) error {
	if req.Disk != "server" {
		return errors.Errorf("unsupported disk: %s, only 'server' disk is supported", req.Disk)
	}

	if req.Clipboard.Disk != "server" {
		return errors.Errorf(
			"unsupported clipboard disk: %s, only same-disk operations are supported",
			req.Clipboard.Disk,
		)
	}

	if req.Clipboard.Type != operationTypeCopy && req.Clipboard.Type != operationTypeCut {
		return errors.Errorf("unsupported clipboard type: %s, must be 'copy' or 'cut'", req.Clipboard.Type)
	}

	if len(req.Clipboard.Files) == 0 && len(req.Clipboard.Directories) == 0 {
		return errors.New("clipboard is empty: no files or directories to paste")
	}

	return nil
}

func (h *Handler) getNode(ctx context.Context, nodeID uint) (*domain.Node, error) {
	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{nodeID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, api.NewNotFoundError("node not found")
	}

	return &nodes[0], nil
}

func (h *Handler) processItems(
	ctx context.Context,
	node *domain.Node,
	serverDir string,
	req *pasteRequest,
) error {
	destPath := strings.ReplaceAll(req.Path, "\\", "/")
	if err := filemanagerpath.ValidatePath(destPath); err != nil {
		return api.WrapHTTPError(err, http.StatusBadRequest)
	}

	destinationBase := filepath.Join(node.WorkPath, serverDir, destPath)

	for _, rawFilePath := range req.Clipboard.Files {
		filePath := strings.ReplaceAll(rawFilePath, "\\", "/")
		if err := filemanagerpath.ValidatePath(filePath); err != nil {
			return api.WrapHTTPError(err, http.StatusBadRequest)
		}

		sourcePath := filepath.Join(node.WorkPath, serverDir, filePath)

		name, err := destinationName(rawFilePath, filePath, req.Names)
		if err != nil {
			return err
		}

		destinationPath := filepath.Join(destinationBase, name)

		if sourcePath == destinationPath {
			if req.Clipboard.Type == operationTypeCut {
				continue
			}

			return api.WrapHTTPError(
				errors.Errorf("copy source and destination are the same: %s", filePath),
				http.StatusBadRequest,
			)
		}

		if err := h.pasteItem(ctx, node, sourcePath, destinationPath, req.Clipboard.Type); err != nil {
			return errors.WithMessagef(err, "failed to paste file: %s", filePath)
		}
	}

	for _, rawDirPath := range req.Clipboard.Directories {
		dirPath := strings.ReplaceAll(rawDirPath, "\\", "/")
		if err := filemanagerpath.ValidatePath(dirPath); err != nil {
			return api.WrapHTTPError(err, http.StatusBadRequest)
		}

		sourcePath := filepath.Join(node.WorkPath, serverDir, dirPath)

		if pathIsInside(destinationBase, sourcePath) {
			return api.WrapHTTPError(
				errors.Errorf("cannot paste directory %s into itself", dirPath),
				http.StatusBadRequest,
			)
		}

		name, err := destinationName(rawDirPath, dirPath, req.Names)
		if err != nil {
			return err
		}

		destinationPath := filepath.Join(destinationBase, name)

		if sourcePath == destinationPath {
			if req.Clipboard.Type == operationTypeCut {
				continue
			}

			return api.WrapHTTPError(
				errors.Errorf("copy source and destination are the same: %s", dirPath),
				http.StatusBadRequest,
			)
		}

		if err := h.pasteItem(ctx, node, sourcePath, destinationPath, req.Clipboard.Type); err != nil {
			return errors.WithMessagef(err, "failed to paste directory: %s", dirPath)
		}
	}

	return nil
}

func (h *Handler) pasteItem(
	ctx context.Context,
	node *domain.Node,
	source string,
	destination string,
	operationType string,
) error {
	switch operationType {
	case operationTypeCopy:
		return h.daemonFiles.Copy(ctx, node, source, destination)
	case operationTypeCut:
		return h.daemonFiles.Move(ctx, node, source, destination)
	default:
		return errors.Errorf("unknown operation type: %s", operationType)
	}
}

// destinationName returns the basename an item is pasted under: the override
// from names (keyed by the raw clipboard entry) or the item's own basename.
func destinationName(rawPath, normalizedPath string, names map[string]string) (string, error) {
	name, ok := names[rawPath]
	if !ok {
		return path.Base(normalizedPath), nil
	}

	if err := filemanagerpath.ValidateFilename(name); err != nil {
		return "", api.WrapHTTPError(err, http.StatusBadRequest)
	}

	return name, nil
}

// pathIsInside reports whether child is parent itself or located inside it.
func pathIsInside(child, parent string) bool {
	return child == parent || strings.HasPrefix(child, parent+string(filepath.Separator))
}
