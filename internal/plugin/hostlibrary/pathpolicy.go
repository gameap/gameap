package hostlibrary

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// PluginServiceRoot is the directory under a node's work path that holds the
// working files of plugins, one subdirectory per plugin.
const PluginServiceRoot = ".plugins"

// PluginServiceDir is where one plugin keeps its own working files on a node:
// <work_path>/.plugins/<compact plugin id>. Request files, staged results and
// anything else a plugin needs a node-side scratch space for belong here, and
// nowhere else — this is the one place outside the game servers' own
// directories that the restricted path policies keep open, and it is opened
// only to the plugin whose id names it.
//
// A plugin id of 0 is a transient load (the upload dry-run), which owns no
// directory: the answer is empty and the caller adds no root.
func PluginServiceDir(workPath string, pluginID uint64) string {
	if pluginID == 0 || strings.TrimSpace(workPath) == "" {
		return ""
	}

	return joinNodePath(workPath, PluginServiceRoot+"/"+pkgplugin.CompactPluginID(domain.Uint64ID(pluginID)))
}

// PathPolicyMode selects which node paths gameap-nodefs (and the work_dir of
// gameap-nodecmd) may name.
type PathPolicyMode string

const (
	// PathPolicyUnrestricted accepts any path, as before the policy existed;
	// only ".." segments and NUL bytes are refused.
	PathPolicyUnrestricted PathPolicyMode = "unrestricted"
	// PathPolicyNodeWorkPath confines paths to the node's work path.
	PathPolicyNodeWorkPath PathPolicyMode = "node_workpath"
	// PathPolicyServerDirs confines paths to the directories of the game
	// servers installed on the node (work path + server dir).
	PathPolicyServerDirs PathPolicyMode = "server_dirs"
)

// ParsePathPolicyMode accepts the configuration value, case-insensitively.
func ParsePathPolicyMode(value string) (PathPolicyMode, error) {
	switch mode := PathPolicyMode(strings.ToLower(strings.TrimSpace(value))); mode {
	case "":
		return PathPolicyUnrestricted, nil
	case PathPolicyUnrestricted, PathPolicyNodeWorkPath, PathPolicyServerDirs:
		return mode, nil
	default:
		return "", errors.Errorf("unknown path policy %q (expected %s, %s or %s)",
			value, PathPolicyUnrestricted, PathPolicyNodeWorkPath, PathPolicyServerDirs)
	}
}

// PathPolicyConfig is what the operator configures.
type PathPolicyConfig struct {
	Mode PathPolicyMode
	// AllowedPaths are additional absolute roots accepted in the restricted
	// modes, on every node.
	AllowedPaths []string
}

// ServerDirLister is the slice of the server repository the server_dirs
// mode needs; satisfied by repositories.ServerRepository.
type ServerDirLister interface {
	Find(
		ctx context.Context,
		filter *filters.FindServer,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Server, error)
}

// PathPolicy decides, per node, which paths a plugin may name.
type PathPolicy struct {
	mode       PathPolicyMode
	extraRoots []string
	servers    ServerDirLister
}

// NewPathPolicy validates the configuration: every extra root must be an
// absolute, clean path. The server lister is only consulted in the
// server_dirs mode and may be nil otherwise.
func NewPathPolicy(cfg PathPolicyConfig, servers ServerDirLister) (*PathPolicy, error) {
	mode, err := ParsePathPolicyMode(string(cfg.Mode))
	if err != nil {
		return nil, err
	}

	policy := &PathPolicy{mode: mode, servers: servers}

	for _, root := range cfg.AllowedPaths {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		normalized, err := normalizeAbsoluteRoot(root)
		if err != nil {
			return nil, errors.WithMessagef(err, "allowed path %q", root)
		}

		policy.extraRoots = append(policy.extraRoots, normalized)
	}

	if mode == PathPolicyServerDirs && servers == nil {
		return nil, errors.New("server_dirs path policy needs the server repository")
	}

	return policy, nil
}

// DefaultPathPolicy is the unrestricted policy.
func DefaultPathPolicy() *PathPolicy {
	return &PathPolicy{mode: PathPolicyUnrestricted}
}

// Mode reports the configured mode.
func (p *PathPolicy) Mode() PathPolicyMode {
	if p == nil {
		return PathPolicyUnrestricted
	}

	return p.mode
}

// PathScope is the set of roots valid for one node during one host call.
type PathScope struct {
	mode     PathPolicyMode
	workPath string
	caseFold bool
	roots    []string
}

// ScopeFor resolves the roots for a node: nothing in the unrestricted mode,
// the work path in node_workpath, the server directories (plus the extra
// roots) in server_dirs. The server lookup happens once per host call, so a
// call naming several paths (copy, hash, archive) pays for it once.
func (p *PathPolicy) ScopeFor(ctx context.Context, node *domain.Node, pluginID uint64) (*PathScope, error) {
	if p == nil || node == nil {
		return &PathScope{mode: PathPolicyUnrestricted}, nil
	}

	scope := &PathScope{mode: p.mode, caseFold: node.OS == domain.NodeOSWindows}

	if p.mode == PathPolicyUnrestricted {
		return scope, nil
	}

	workPath, err := normalizeAbsoluteRoot(node.WorkPath)
	if err != nil {
		return nil, errors.New("node work path is not configured")
	}

	scope.workPath = workPath
	scope.roots = append(scope.roots, p.extraRoots...)

	// Every restricted mode keeps the plugin's own service directory open.
	// Without it a plugin cannot stage so much as a request file, and the
	// operator would have to name each plugin's directory by hand — as an
	// absolute path, on every node, which is not something the work paths of a
	// mixed fleet allow.
	if serviceDir := PluginServiceDir(workPath, pluginID); serviceDir != "" {
		scope.roots = append(scope.roots, serviceDir)
	}

	switch p.mode {
	case PathPolicyNodeWorkPath:
		scope.roots = append(scope.roots, workPath)
	case PathPolicyServerDirs:
		servers, err := p.servers.Find(ctx, filters.FindServerByNodeIDs(node.ID), nil, nil)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to list the node's servers")
		}

		for _, server := range servers {
			if strings.TrimSpace(server.Dir) == "" {
				continue
			}

			scope.roots = append(scope.roots, joinNodePath(workPath, server.Dir))
		}
	case PathPolicyUnrestricted:
	}

	return scope, nil
}

// PathPolicyError explains a refused path; its text is stable so plugins
// can recognise it by the "path policy:" prefix.
type PathPolicyError struct {
	Path   string
	Mode   PathPolicyMode
	Reason string
}

func (e *PathPolicyError) Error() string {
	return "path policy: " + e.Reason + ": " + e.Path
}

// Check validates one nodefs path against the scope.
func (s *PathScope) Check(raw string) *PathPolicyError {
	normalized, err := NormalizeNodePath(raw, s.workPath)
	if err != nil {
		return &PathPolicyError{Path: raw, Mode: s.mode, Reason: err.Error()}
	}

	if s.mode == PathPolicyUnrestricted {
		return nil
	}

	if !s.inRoots(normalized) {
		return &PathPolicyError{Path: raw, Mode: s.mode, Reason: "path outside allowed roots (" + string(s.mode) + ")"}
	}

	return nil
}

// CheckWorkDir validates the working directory of a node command: the same
// rules as Check, and the restricted modes additionally require an absolute
// path, because a relative one is resolved by the daemon against whatever
// its own working directory is.
func (s *PathScope) CheckWorkDir(raw string) *PathPolicyError {
	if s.mode != PathPolicyUnrestricted && !isAbsoluteNodePath(raw) {
		return &PathPolicyError{Path: raw, Mode: s.mode, Reason: "path must be absolute"}
	}

	return s.Check(raw)
}

func (s *PathScope) inRoots(normalized string) bool {
	candidate := normalized
	if s.caseFold {
		candidate = strings.ToLower(candidate)
	}

	for _, root := range s.roots {
		if s.caseFold {
			root = strings.ToLower(root)
		}

		if candidate == root || strings.HasPrefix(candidate, strings.TrimSuffix(root, "/")+"/") {
			return true
		}
	}

	return false
}

// NormalizeNodePath brings a path the way a plugin sends it into the form
// the roots are compared in: backslashes become slashes, a relative path is
// placed under the work path (when one is known), the result is cleaned. A
// NUL byte or a ".." segment is refused before cleaning, because cleaning
// through a symbolic link is exactly how a path escapes its root.
func NormalizeNodePath(raw, workPath string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", errors.New("path contains a null byte")
	}

	normalized := strings.ReplaceAll(raw, `\`, "/")

	if slices.Contains(strings.Split(normalized, "/"), "..") {
		return "", errors.New(`path contains ".." segment`)
	}

	if !isAbsoluteNodePath(normalized) && workPath != "" {
		normalized = joinNodePath(workPath, normalized)
	}

	return cleanNodePath(normalized), nil
}

// isAbsoluteNodePath accepts Unix absolute paths and Windows drive paths.
func isAbsoluteNodePath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(p, "/") {
		return true
	}

	return len(p) >= 3 && p[1] == ':' && p[2] == '/' && isDriveLetter(p[0])
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// cleanNodePath cleans a slash-separated path, keeping a Windows drive
// prefix upper-cased and dropping a trailing slash (except for a root).
func cleanNodePath(p string) string {
	if len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0]) {
		rest := path.Clean("/" + strings.TrimPrefix(p[2:], "/"))

		return strings.ToUpper(p[:1]) + ":" + rest
	}

	return path.Clean(p)
}

func joinNodePath(root, rel string) string {
	rel = strings.ReplaceAll(rel, `\`, "/")

	return cleanNodePath(strings.TrimSuffix(root, "/") + "/" + strings.TrimPrefix(rel, "/"))
}

// normalizeAbsoluteRoot prepares a configured or node-provided root.
func normalizeAbsoluteRoot(root string) (string, error) {
	normalized, err := NormalizeNodePath(root, "")
	if err != nil {
		return "", err
	}

	if !isAbsoluteNodePath(normalized) {
		return "", errors.New("path must be absolute")
	}

	return normalized, nil
}
