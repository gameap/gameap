// Package pluginssh runs SSH connections and remote commands on behalf of wasm
// plugins and delivers completion events back into them.
//
// Sessions are created from gameap-ssh host functions, which may run while the
// plugin manager lock is held (guest Initialize/Shutdown), so this path never
// resolves a plugin: the callback goroutine looks it up at delivery time.
package pluginssh

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/pkg/errors"
)

// Service owns the shared SSH machinery: dial policy, limits and the delivery
// of completion callbacks. Per-plugin state lives in Sessions.
type Service struct {
	plugins      PluginProvider
	idResolver   ManagerIDResolver
	cfg          Config
	logger       *slog.Logger
	resolver     netResolver
	dialer       netDialer
	allowedHosts map[string]struct{}

	mu       sync.Mutex
	baseCtx  context.Context
	stopped  bool
	sessions map[*Sessions]struct{}

	deliveryWG sync.WaitGroup
}

func New(plugins PluginProvider, idResolver ManagerIDResolver, cfg Config, logger *slog.Logger) *Service {
	return newService(plugins, idResolver, cfg, logger, net.DefaultResolver, &net.Dialer{})
}

func newService(
	plugins PluginProvider,
	idResolver ManagerIDResolver,
	cfg Config,
	logger *slog.Logger,
	resolver netResolver,
	dialer netDialer,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	cfg = cfg.withDefaults()

	allowed := make(map[string]struct{}, len(cfg.AllowedHosts))
	for _, host := range cfg.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			allowed[host] = struct{}{}
		}
	}

	return &Service{
		plugins:      plugins,
		idResolver:   idResolver,
		cfg:          cfg,
		logger:       logger,
		resolver:     resolver,
		dialer:       dialer,
		allowedHosts: allowed,
		baseCtx:      context.Background(),
		sessions:     make(map[*Sessions]struct{}),
	}
}

// Start records the lifetime context used for callback delivery. Guest calls
// are detached from it: a completion must survive the request that started it.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.baseCtx = ctx
}

// Stop closes every live session set and waits for in-flight callbacks, so no
// guest call is running when the plugin runtimes are closed.
func (s *Service) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()

		return
	}
	s.stopped = true

	live := make([]*Sessions, 0, len(s.sessions))
	for sessions := range s.sessions {
		live = append(live, sessions)
	}
	s.mu.Unlock()

	for _, sessions := range live {
		sessions.Close()
	}

	s.deliveryWG.Wait()
}

// NewSessions hands out the per-plugin session set backing one gameap-ssh
// module instance. Closing the instance releases everything it held.
func (s *Service) NewSessions(pluginID uint64) *Sessions {
	sessions := newSessions(s, pluginID)

	s.mu.Lock()
	stopped := s.stopped
	if !stopped {
		s.sessions[sessions] = struct{}{}
	}
	s.mu.Unlock()

	// A plugin loading after Stop (a shutdown racing an install) gets a set
	// that refuses everything. Closing it happens outside the lock: Close
	// deregisters through forget, which takes the same lock.
	if stopped {
		sessions.Close()
	}

	return sessions
}

func (s *Service) forget(sessions *Sessions) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessions)
}

func (s *Service) context() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.baseCtx
}

// notifyCompleted schedules the completion callback. It is called from the
// operation goroutine, never from a host call holding a lock.
func (s *Service) notifyCompleted(sessions *Sessions, op *operation) {
	if sessionsClosed(sessions) {
		return
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()

		return
	}
	s.deliveryWG.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.deliveryWG.Done()

		s.deliverCompleted(sessions, op)
	}()
}

func (s *Service) deliverCompleted(sessions *Sessions, op *operation) {
	if sessionsClosed(sessions) {
		return
	}

	handler, plugin, ok := s.resolveHandler(sessions.pluginID)
	if !ok {
		return
	}

	request := op.completionRequest()

	for attempt := 0; attempt <= s.cfg.BusyRetries; attempt++ {
		// resolveHandler finds the plugin by its database id at delivery time,
		// so once this set is closed the id may already belong to a reloaded
		// instance that never started the operation.
		if sessionsClosed(sessions) {
			return
		}

		callCtx, cancel := context.WithTimeout(
			context.WithoutCancel(s.context()),
			s.cfg.CompletionCallTimeout,
		)

		_, err := handler.HandleExecCompleted(callCtx, request)
		timedOut := callCtx.Err() != nil
		cancel()

		if err == nil {
			return
		}

		if errors.Is(err, pkgplugin.ErrPluginBusy) {
			if !s.waitBeforeRetry(sessions) {
				return
			}

			continue
		}

		s.handleGuestCallError(plugin, op, err, timedOut)

		return
	}

	s.logger.Error("plugin stayed busy, ssh exec completion callback dropped",
		slog.Uint64("plugin_id", sessions.pluginID),
		slog.String("operation_id", op.id))
}

func sessionsClosed(sessions *Sessions) bool {
	select {
	case <-sessions.closedCh:
		return true
	default:
		return false
	}
}

// waitBeforeRetry pauses between busy retries, giving up early when the plugin
// goes away or the panel shuts down.
func (s *Service) waitBeforeRetry(sessions *Sessions) bool {
	timer := time.NewTimer(s.cfg.BusyRetryDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-sessions.closedCh:
		return false
	case <-s.context().Done():
		return false
	}
}

func (s *Service) handleGuestCallError(plugin *pkgplugin.LoadedPlugin, op *operation, err error, timedOut bool) {
	if timedOut {
		// A guest that cannot answer within the call budget has wedged its
		// runtime; disabling it stops the panel from feeding it more work.
		plugin.Disable()
		s.logger.Error("ssh exec completion callback timed out, plugin disabled until reload",
			slog.String("operation_id", op.id),
			slog.String("error", err.Error()))

		return
	}

	s.logger.Warn("ssh exec completion callback failed",
		slog.String("operation_id", op.id),
		slog.String("error", err.Error()))
}

func (s *Service) resolveHandler(pluginID uint64) (sshsdk.SSHExecEventsHandler, *pkgplugin.LoadedPlugin, bool) {
	if s.plugins == nil {
		return nil, nil, false
	}

	managerID := pkgplugin.CompactPluginID(domain.Uint64ID(pluginID))
	if s.idResolver != nil {
		if resolved, ok := s.idResolver.GetPluginManagerID(domain.Uint64ID(pluginID)); ok {
			managerID = resolved
		}
	}

	plugin, ok := s.plugins.GetPlugin(managerID)
	if !ok || !plugin.IsEnabled() || !plugin.HasSSHExecEventsHandler() {
		return nil, nil, false
	}

	handler, ok := plugin.Instance.(sshsdk.SSHExecEventsHandler)
	if !ok {
		return nil, nil, false
	}

	return handler, plugin, true
}
