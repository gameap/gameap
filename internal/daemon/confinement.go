package daemon

import (
	"context"
	"path/filepath"

	"github.com/gameap/gameap/internal/domain"
)

// CapabilityFileConfinement is advertised by a daemon that keeps a file request
// inside the game server directory it belongs to and honours the root a request
// names. A daemon without it confines to the node work directory only, where a
// symlink leads from one game server into another.
const CapabilityFileConfinement = "file_confinement"

type confinementRootKey struct{}

// ConfineToServer returns a context whose file, transfer and archive requests
// the daemon keeps inside the directory of server on node.
//
// Validating a path in the panel cannot do that: a path that looks like it
// stays below the server directory still leaves it through a symlink, and
// symlinks live on the node. The files of a game server are written by its
// tenant, so everything done on a tenant's behalf has to carry the boundary.
func ConfineToServer(ctx context.Context, node *domain.Node, server *domain.Server) context.Context {
	return WithConfinementRoot(ctx, stripWorkPath(node.WorkPath, filepath.Join(node.WorkPath, server.Dir)))
}

// WithConfinementRoot is ConfineToServer for a root that is already relative to
// the node work directory. An empty root adds nothing: the daemon is left with
// the game server directories it knows.
func WithConfinementRoot(ctx context.Context, root string) context.Context {
	if root == "" {
		return ctx
	}

	return context.WithValue(ctx, confinementRootKey{}, root)
}

// ConfinementRoot returns the root the requests made with ctx are confined to,
// relative to the node work directory, or "" when ctx names none.
func ConfinementRoot(ctx context.Context) string {
	root, _ := ctx.Value(confinementRootKey{}).(string)

	return root
}
