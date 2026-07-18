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

	sourceBase := filepath.Join(node.WorkPath, serverDir)
	destinationBase := filepath.Join(node.WorkPath, serverDir, destPath)

	operations := make([]pasteOperation, 0, len(req.Clipboard.Files)+len(req.Clipboard.Directories))

	for _, rawFilePath := range req.Clipboard.Files {
		operation, ok, err := planPasteItem(sourceBase, destinationBase, rawFilePath, false, req)
		if err != nil {
			return err
		}

		if ok {
			operations = append(operations, operation)
		}
	}

	for _, rawDirPath := range req.Clipboard.Directories {
		operation, ok, err := planPasteItem(sourceBase, destinationBase, rawDirPath, true, req)
		if err != nil {
			return err
		}

		if ok {
			operations = append(operations, operation)
		}
	}

	for _, operation := range operations {
		if err := h.pasteItem(ctx, node, operation.source, operation.destination, req.Clipboard.Type); err != nil {
			itemKind := "file"
			if operation.isDir {
				itemKind = "directory"
			}

			return errors.WithMessagef(err, "failed to paste %s: %s", itemKind, operation.itemPath)
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

// pasteOperation is a validated copy/move planned for execution. The whole
// request is planned before the first daemon call so a validation error in
// any item rejects the batch without partially applying it.
type pasteOperation struct {
	source      string
	destination string
	itemPath    string
	isDir       bool
}

// planPasteItem validates one clipboard item and resolves its operation.
// ok is false for a same-path move, which is skipped as a no-op.
func planPasteItem(
	sourceBase string,
	destinationBase string,
	rawPath string,
	isDir bool,
	req *pasteRequest,
) (pasteOperation, bool, error) {
	itemPath := strings.ReplaceAll(rawPath, "\\", "/")
	if err := filemanagerpath.ValidatePath(itemPath); err != nil {
		return pasteOperation{}, false, api.WrapHTTPError(err, http.StatusBadRequest)
	}

	sourcePath := filepath.Join(sourceBase, itemPath)

	if isDir && pathIsInside(destinationBase, sourcePath) {
		return pasteOperation{}, false, api.WrapHTTPError(
			errors.Errorf("cannot paste directory %s into itself", itemPath),
			http.StatusBadRequest,
		)
	}

	name, err := destinationName(rawPath, itemPath, req.Names)
	if err != nil {
		return pasteOperation{}, false, err
	}

	destinationPath := filepath.Join(destinationBase, name)

	if sourcePath == destinationPath {
		if req.Clipboard.Type == operationTypeCut {
			return pasteOperation{}, false, nil
		}

		return pasteOperation{}, false, api.WrapHTTPError(
			errors.Errorf("copy source and destination are the same: %s", itemPath),
			http.StatusBadRequest,
		)
	}

	operation := pasteOperation{
		source:      sourcePath,
		destination: destinationPath,
		itemPath:    itemPath,
		isDir:       isDir,
	}

	return operation, true, nil
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
