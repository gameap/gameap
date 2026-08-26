package hostlibrary

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/services/pluginssh"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
)

// sshDefaultPort mirrors the port the session layer dials when the request
// names none; the audit record names the endpoint actually used.
const sshDefaultPort = 22

// errDeadlineTooClose is reported when the guest call is about to expire: the
// host answers instead of starting work it could not finish.
var errDeadlineTooClose = errors.New("guest call deadline is too close to run this operation")

// errRemoteFileCommandFailed marks a refused remote file transfer for the
// audit record; the plugin sees the exit code and stderr, the audit stream
// only needs the outcome.
var errRemoteFileCommandFailed = errors.New("remote file command failed")

// SSHServiceImpl implements sshsdk.SSHService for a single plugin, delegating
// the actual SSH work to the session set the factory bound to it.
type SSHServiceImpl struct {
	sessions SSHSessionManager
	guard    *PluginGuard
}

func NewSSHService(sessions SSHSessionManager, guard *PluginGuard) *SSHServiceImpl {
	return &SSHServiceImpl{
		sessions: sessions,
		guard:    guard,
	}
}

// sshTargetResourceID names the endpoint of a connection for the audit
// resource_id.
func sshTargetResourceID(host string, port uint32) string {
	if port == 0 {
		port = sshDefaultPort
	}

	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
}

// sshHandleResourceID names an open connection for the audit resource_id.
func sshHandleResourceID(handle uint64) string {
	return strconv.FormatUint(handle, 10)
}

// auditExec records a command run on a plugin-named host. The command text and
// its stdin never reach the audit stream: they routinely carry credentials the
// plugin is bootstrapping the machine with.
func (s *SSHServiceImpl) auditExec(ctx context.Context, action string, handle uint64, operationID string, err error) {
	extra := make([]slog.Attr, 0, 2)
	if operationID != "" {
		extra = append(extra, slog.String("operation_id", operationID))
	}
	if host, ok := s.sessions.ConnectionHost(handle); ok {
		extra = append(extra, slog.String("host", host))
	}

	s.guard.Audit(ctx, audit.EventPluginSSHExec, action,
		"ssh_session", sshHandleResourceID(handle), err, extra...)
}

// auditFile records a remote file transfer: the path it touched and the
// connection it used, never the content that moved over it.
func (s *SSHServiceImpl) auditFile(ctx context.Context, action string, handle uint64, path string, err error) {
	extra := make([]slog.Attr, 0, 2)
	extra = append(extra, slog.String("path", path))
	if host, ok := s.sessions.ConnectionHost(handle); ok {
		extra = append(extra, slog.String("host", host))
	}

	s.guard.Audit(ctx, audit.EventPluginSSHFile, action,
		"ssh_session", sshHandleResourceID(handle), err, extra...)
}

func (s *SSHServiceImpl) GenerateKeyPair(
	ctx context.Context,
	req *sshsdk.GenerateKeyPairRequest,
) (*sshsdk.GenerateKeyPairResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "generate_key_pair"); msg != "" {
		return &sshsdk.GenerateKeyPairResponse{Error: new(msg)}, nil
	}

	pair, err := pluginssh.GenerateKeyPair(keyTypeFromProto(req.Type), req.Comment)
	if err != nil {
		s.guard.Audit(ctx, audit.EventPluginSSHKey, "generate_key_pair", "ssh_key", "", err)

		return &sshsdk.GenerateKeyPairResponse{Error: new(err.Error())}, nil
	}

	// The fingerprint is what ties an authorized_keys line found on a machine
	// back to the plugin that minted it; the key material itself never enters
	// the record.
	s.guard.Audit(ctx, audit.EventPluginSSHKey, "generate_key_pair",
		"ssh_key", pair.FingerprintSHA256, nil,
		slog.String("key_type", pair.KeyType))

	return &sshsdk.GenerateKeyPairResponse{
		Success:           true,
		PrivateKeyPem:     pair.PrivateKeyPEM,
		PublicKey:         pair.PublicKey,
		FingerprintSha256: pair.FingerprintSHA256,
		KeyType:           pair.KeyType,
	}, nil
}

func (s *SSHServiceImpl) Connect(
	ctx context.Context,
	req *sshsdk.ConnectRequest,
) (*sshsdk.ConnectResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "connect"); msg != "" {
		return &sshsdk.ConnectResponse{Error: new(msg)}, nil
	}

	budget := syncWaitBudget(ctx, time.Duration(req.ConnectTimeoutMs)*time.Millisecond)
	if budget <= 0 {
		return &sshsdk.ConnectResponse{Error: new(errDeadlineTooClose.Error())}, nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	result, err := s.sessions.Connect(connectCtx, connectParamsFromProto(req))
	if err != nil {
		s.guard.Audit(ctx, audit.EventPluginSSHConnect, "connect",
			"ssh_host", sshTargetResourceID(req.Host, req.Port), err,
			slog.String("user", req.User))

		response := &sshsdk.ConnectResponse{Error: new(err.Error())}

		// A rejected host key is the one failure where the plugin needs to see
		// what actually answered, so it can pin or report the key.
		var rejected *pluginssh.HostKeyRejectedError
		if errors.As(err, &rejected) {
			response.HostKeyFingerprintSha256 = rejected.FingerprintSHA256
			response.HostKeyType = rejected.KeyType
		}

		return response, nil
	}

	s.guard.Audit(ctx, audit.EventPluginSSHConnect, "connect",
		"ssh_host", sshTargetResourceID(req.Host, req.Port), nil,
		slog.String("user", req.User),
		slog.String("host_key_type", result.HostKeyType),
		slog.String("host_key_fingerprint", result.HostKeyFingerprintSHA256),
		slog.Bool("host_key_verified", result.HostKeyVerified),
		slog.String("handle", sshHandleResourceID(result.Handle)),
		slog.String("address", result.Address))

	return &sshsdk.ConnectResponse{
		Success:                  true,
		Handle:                   result.Handle,
		HostKeyFingerprintSha256: result.HostKeyFingerprintSHA256,
		HostKeyType:              result.HostKeyType,
		ServerVersion:            result.ServerVersion,
	}, nil
}

func (s *SSHServiceImpl) Disconnect(
	ctx context.Context,
	req *sshsdk.DisconnectRequest,
) (*sshsdk.DisconnectResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "disconnect"); msg != "" {
		return &sshsdk.DisconnectResponse{Error: new(msg)}, nil
	}

	if err := s.sessions.Disconnect(req.Handle); err != nil {
		return &sshsdk.DisconnectResponse{Error: new(err.Error())}, nil
	}

	return &sshsdk.DisconnectResponse{Success: true}, nil
}

// Exec runs a command and waits for it within the budget. When the budget runs
// out the command keeps running and the plugin is subscribed to the completion
// callback, so a long bootstrap is not lost just because the guest call was
// shorter than the work.
func (s *SSHServiceImpl) Exec(
	ctx context.Context,
	req *sshsdk.ExecRequest,
) (*sshsdk.ExecSyncResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "exec"); msg != "" {
		return &sshsdk.ExecSyncResponse{Error: new(msg)}, nil
	}

	params := execParamsFromProto(req)
	params.NotifyCompletion = false

	operationID, err := s.sessions.StartExec(ctx, params)
	if err != nil {
		s.auditExec(ctx, "exec", req.Handle, "", err)

		return &sshsdk.ExecSyncResponse{Error: new(err.Error())}, nil
	}

	s.auditExec(ctx, "exec", req.Handle, operationID, nil)

	response := &sshsdk.ExecSyncResponse{
		Success:     true,
		OperationId: operationID,
		Status:      sshsdk.ExecStatus_EXEC_STATUS_RUNNING,
		ExitCode:    -1,
	}

	budget := syncWaitBudget(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	if budget <= 0 {
		s.subscribeCompletion(ctx, operationID)

		return response, nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	if err := s.sessions.WaitCompletion(waitCtx, operationID); err != nil {
		s.subscribeCompletion(ctx, operationID)

		return response, nil
	}

	snapshot, ok := s.sessions.Snapshot(operationID, 0, 0)
	if !ok {
		return response, nil
	}

	return execSyncResponseFromSnapshot(snapshot), nil
}

// subscribeCompletion registers callback interest for a command that outlived
// its blocking call; a plugin without the handler export simply never hears
// back and can poll instead.
func (s *SSHServiceImpl) subscribeCompletion(ctx context.Context, operationID string) {
	if err := s.sessions.SubscribeCompletion(operationID); err != nil {
		slog.DebugContext(ctx, "failed to subscribe to ssh exec completion",
			slog.Uint64("plugin_id", s.guard.PluginID()),
			slog.String("operation_id", operationID),
			slog.String("error", err.Error()))
	}
}

func (s *SSHServiceImpl) StartExec(
	ctx context.Context,
	req *sshsdk.ExecRequest,
) (*sshsdk.StartExecResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "start_exec"); msg != "" {
		return &sshsdk.StartExecResponse{Error: new(msg)}, nil
	}

	params := execParamsFromProto(req)
	params.NotifyCompletion = true

	operationID, err := s.sessions.StartExec(ctx, params)
	if err != nil {
		s.auditExec(ctx, "start_exec", req.Handle, "", err)

		return &sshsdk.StartExecResponse{Error: new(err.Error())}, nil
	}

	s.auditExec(ctx, "start_exec", req.Handle, operationID, nil)

	return &sshsdk.StartExecResponse{Success: true, OperationId: operationID}, nil
}

func (s *SSHServiceImpl) GetExecOperation(
	ctx context.Context,
	req *sshsdk.GetExecOperationRequest,
) (*sshsdk.GetExecOperationResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "get_exec_operation"); msg != "" {
		return &sshsdk.GetExecOperationResponse{Error: new(msg)}, nil
	}

	if req.WaitMs > 0 {
		if budget := syncWaitBudget(ctx, time.Duration(req.WaitMs)*time.Millisecond); budget > 0 {
			waitCtx, cancel := context.WithTimeout(ctx, budget)
			_ = s.sessions.WaitCompletion(waitCtx, req.OperationId)
			cancel()
		}
	}

	snapshot, ok := s.sessions.Snapshot(req.OperationId, req.StdoutOffset, req.StderrOffset)
	if !ok {
		return &sshsdk.GetExecOperationResponse{Success: true, Found: false}, nil
	}

	response := &sshsdk.GetExecOperationResponse{
		Success:          true,
		Found:            true,
		Status:           execStatusToProto(snapshot.Status),
		Handle:           snapshot.Handle,
		Stdout:           snapshot.Stdout,
		Stderr:           snapshot.Stderr,
		StdoutNextOffset: snapshot.StdoutNextOffset,
		StderrNextOffset: snapshot.StderrNextOffset,
		StdoutTruncated:  snapshot.StdoutTruncated,
		StderrTruncated:  snapshot.StderrTruncated,
		StdoutTotalBytes: snapshot.StdoutTotal,
		StderrTotalBytes: snapshot.StderrTotal,
		OpSuccess:        snapshot.Succeeded(),
		ExitCode:         snapshot.ExitCode,
		ExitSignal:       snapshot.ExitSignal,
		StartedAt:        unixMilli(snapshot.StartedAt),
		FinishedAt:       unixMilli(snapshot.FinishedAt),
	}

	if snapshot.Error != "" {
		response.OpError = new(snapshot.Error)
	}

	return response, nil
}

func (s *SSHServiceImpl) CancelExec(
	ctx context.Context,
	req *sshsdk.CancelExecRequest,
) (*sshsdk.CancelExecResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "cancel_exec"); msg != "" {
		return &sshsdk.CancelExecResponse{Error: new(msg)}, nil
	}

	err := s.sessions.Cancel(req.OperationId, req.Reason)

	s.guard.Audit(ctx, audit.EventPluginSSHExec, "cancel_exec",
		"ssh_operation", req.OperationId, err)

	if err != nil {
		return &sshsdk.CancelExecResponse{Error: new(err.Error())}, nil
	}

	return &sshsdk.CancelExecResponse{Success: true}, nil
}

// WriteFile pipes the content into a remote `cat`, which avoids requiring an
// SFTP subsystem on a freshly installed machine.
func (s *SSHServiceImpl) WriteFile(
	ctx context.Context,
	req *sshsdk.WriteFileRequest,
) (*sshsdk.WriteFileResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "write_file"); msg != "" {
		return &sshsdk.WriteFileResponse{Error: new(msg)}, nil
	}

	command, err := writeFileCommand(req.Path, req.Mode)
	if err != nil {
		return &sshsdk.WriteFileResponse{Error: new(err.Error())}, nil
	}

	snapshot, err := s.runBlocking(ctx, &sshsdk.ExecRequest{
		Handle:         req.Handle,
		Command:        command,
		Stdin:          req.Content,
		TimeoutSeconds: req.TimeoutSeconds,
	})
	if err != nil {
		s.auditFile(ctx, "write_file", req.Handle, req.Path, err)

		return &sshsdk.WriteFileResponse{Error: new(err.Error())}, nil
	}

	if !snapshot.Succeeded() {
		s.auditFile(ctx, "write_file", req.Handle, req.Path, errRemoteFileCommandFailed)

		return &sshsdk.WriteFileResponse{
			Error:  new(remoteFailureMessage(snapshot)),
			Stderr: string(snapshot.Stderr),
		}, nil
	}

	s.auditFile(ctx, "write_file", req.Handle, req.Path, nil)

	return &sshsdk.WriteFileResponse{Success: true, Stderr: string(snapshot.Stderr)}, nil
}

func (s *SSHServiceImpl) ReadFile(
	ctx context.Context,
	req *sshsdk.ReadFileRequest,
) (*sshsdk.ReadFileResponse, error) {
	if msg := s.guard.Check(ctx, ModuleSSH, "read_file"); msg != "" {
		return &sshsdk.ReadFileResponse{Error: new(msg)}, nil
	}

	command, err := readFileCommand(req.Path)
	if err != nil {
		return &sshsdk.ReadFileResponse{Error: new(err.Error())}, nil
	}

	snapshot, err := s.runBlocking(ctx, &sshsdk.ExecRequest{
		Handle:         req.Handle,
		Command:        command,
		TimeoutSeconds: req.TimeoutSeconds,
		MaxOutputBytes: req.MaxBytes,
	})
	if err != nil {
		s.auditFile(ctx, "read_file", req.Handle, req.Path, err)

		return &sshsdk.ReadFileResponse{Error: new(err.Error())}, nil
	}

	if !snapshot.Succeeded() {
		s.auditFile(ctx, "read_file", req.Handle, req.Path, errRemoteFileCommandFailed)

		return &sshsdk.ReadFileResponse{
			Error:  new(remoteFailureMessage(snapshot)),
			Stderr: string(snapshot.Stderr),
		}, nil
	}

	s.auditFile(ctx, "read_file", req.Handle, req.Path, nil)

	return &sshsdk.ReadFileResponse{
		Success:   true,
		Content:   snapshot.Stdout,
		Truncated: snapshot.StdoutTruncated,
		Stderr:    string(snapshot.Stderr),
	}, nil
}

// runBlocking executes a command and waits for it inside the guest deadline.
// The file helpers are always blocking: a half-written file is not something a
// plugin can usefully poll for.
func (s *SSHServiceImpl) runBlocking(
	ctx context.Context,
	req *sshsdk.ExecRequest,
) (pluginssh.ExecSnapshot, error) {
	budget := syncWaitBudget(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	if budget <= 0 {
		return pluginssh.ExecSnapshot{}, errDeadlineTooClose
	}

	operationID, err := s.sessions.StartExec(ctx, execParamsFromProto(req))
	if err != nil {
		return pluginssh.ExecSnapshot{}, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	if err := s.sessions.WaitCompletion(waitCtx, operationID); err != nil {
		_ = s.sessions.Cancel(operationID, "guest call deadline reached")

		return pluginssh.ExecSnapshot{}, errors.WithMessage(err, "remote command did not finish in time")
	}

	snapshot, ok := s.sessions.Snapshot(operationID, 0, 0)
	if !ok {
		return pluginssh.ExecSnapshot{}, pluginssh.ErrOperationNotFound
	}

	return snapshot, nil
}

// SSHHostLibrary wires a per-plugin gameap-ssh module into a wazero runtime.
type SSHHostLibrary struct {
	impl *SSHServiceImpl
}

func (l *SSHHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return sshsdk.Instantiate(ctx, r, l.impl)
}

// Close releases the plugin's connections and running commands. The manager
// calls it right after closing the runtime, so an unloaded plugin leaves no
// open SSH session behind.
func (l *SSHHostLibrary) Close(_ context.Context) error {
	l.impl.sessions.Close()

	return nil
}

var _ pkgplugin.HostLibraryCloser = (*SSHHostLibrary)(nil)

// SSHHostLibraryFactory builds a gameap-ssh library bound to each plugin's ID:
// the module is gated on the plugin's own ssh grant, rate limited and audited
// through the shared guard, and its connections are released when that plugin
// is unloaded.
type SSHHostLibraryFactory struct {
	opener SSHSessionOpener
	guard  *Guard
}

func NewSSHHostLibraryFactory(opener SSHSessionOpener, guard *Guard) *SSHHostLibraryFactory {
	return &SSHHostLibraryFactory{opener: opener, guard: guard}
}

func (f *SSHHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &SSHHostLibrary{
		impl: NewSSHService(f.opener.NewSessions(pluginID), f.guard.For(pluginID)),
	}
}
