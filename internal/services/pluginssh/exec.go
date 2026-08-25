package pluginssh

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/xid"
	"golang.org/x/crypto/ssh"
)

// operation is one remote command and everything observers need after it ends.
type operation struct {
	id     string
	handle uint64

	stdout *capturedStream
	stderr *capturedStream

	cancelFn context.CancelCauseFunc
	done     chan struct{}

	mu         sync.Mutex
	status     Status
	exitCode   int32
	exitSignal string
	errMessage string
	notify     bool
	startedAt  time.Time
	finishedAt time.Time
}

// StartExec launches a command and returns its operation id. The command runs
// independently of the guest call that started it, so a slow bootstrap does not
// hold the plugin's call gate.
func (p *Sessions) StartExec(_ context.Context, params ExecParams) (string, error) {
	if err := p.validateExec(params); err != nil {
		return "", err
	}

	conn, err := p.connection(params.Handle)
	if err != nil {
		return "", err
	}

	if err := p.reserveOperationSlot(); err != nil {
		return "", err
	}

	session, err := p.prepareSession(conn, params)
	if err != nil {
		p.releaseOperationSlot()

		return "", err
	}

	op, err := p.registerOperation(conn, session, params)
	if err != nil {
		return "", err
	}

	return op.id, nil
}

func (p *Sessions) validateExec(params ExecParams) error {
	if strings.TrimSpace(params.Command) == "" {
		return ErrCommandRequired
	}

	if len(params.Command) > maxCommandBytes {
		return ErrCommandTooLong
	}

	if len(params.Stdin) > p.svc.cfg.MaxStdinBytes {
		return ErrStdinTooLarge
	}

	if len(params.Env) > maxEnvVars {
		return ErrTooManyEnvVars
	}

	for name := range params.Env {
		if name == "" || strings.ContainsAny(name, "=\x00") {
			return errors.WithMessage(ErrInvalidEnvName, name)
		}
	}

	return nil
}

func (p *Sessions) reserveOperationSlot() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrSessionsClosed
	}

	if p.running >= p.svc.cfg.MaxOperations {
		return ErrTooManyOperations
	}

	p.running++

	return nil
}

func (p *Sessions) releaseOperationSlot() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running > 0 {
		p.running--
	}
}

// prepareSession opens the channel, applies env and starts the command. A
// failure here produces no operation record: nothing ran.
func (p *Sessions) prepareSession(conn *connection, params ExecParams) (*ssh.Session, error) {
	session, err := conn.client.NewSession()
	if err != nil {
		return nil, errors.Wrap(ErrStartFailed, err.Error())
	}

	for name, value := range params.Env {
		if err := session.Setenv(name, value); err != nil {
			_ = session.Close()

			return nil, errors.WithMessage(ErrEnvRejected, name)
		}
	}

	outputCap := clampBytes(params.MaxOutputBytes, p.svc.cfg.MaxOutputBytes)
	session.Stdout = newCapturedStream(outputCap)
	session.Stderr = newCapturedStream(outputCap)

	if len(params.Stdin) > 0 {
		session.Stdin = bytes.NewReader(params.Stdin)
	}

	if err := session.Start(params.Command); err != nil {
		_ = session.Close()

		return nil, errors.Wrap(ErrStartFailed, err.Error())
	}

	return session, nil
}

// registerOperation records the running command and spawns the goroutine that
// waits for it, enforces the timeout and publishes the outcome. Sessions may
// have been closed while the session was being prepared: the already-started
// command is then torn down instead of surviving in a closed set.
func (p *Sessions) registerOperation(conn *connection, session *ssh.Session, params ExecParams) (*operation, error) {
	timeout := clampDuration(params.Timeout, p.svc.cfg.MaxExecTimeout)

	// Detached from the guest call: the command outlives the host call that
	// started it, and only its own timeout stops it.
	opCtx, cancel := context.WithCancelCause(context.WithoutCancel(p.svc.context()))
	opCtx, cancelTimeout := context.WithTimeoutCause(opCtx, timeout, errExecTimeout)

	op := &operation{
		id:        xid.New().String(),
		handle:    params.Handle,
		stdout:    session.Stdout.(*capturedStream), //nolint:forcetypeassert // set in prepareSession
		stderr:    session.Stderr.(*capturedStream), //nolint:forcetypeassert // set in prepareSession
		cancelFn:  cancel,
		done:      make(chan struct{}),
		status:    StatusRunning,
		exitCode:  -1,
		notify:    params.NotifyCompletion,
		startedAt: time.Now(),
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()

		cancel(&cancelCauseError{reason: "plugin unloaded"})
		cancelTimeout()
		_ = session.Close()
		p.releaseOperationSlot()

		return nil, ErrSessionsClosed
	}
	p.ops[op.id] = op
	p.mu.Unlock()

	conn.touch()

	// Killing the remote command on cancellation: the signal is best-effort
	// (many servers ignore it), closing the session is what always lands.
	stopKiller := context.AfterFunc(opCtx, func() {
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
	})

	go func() {
		defer cancelTimeout()

		waitErr := session.Wait()
		stopKiller()
		_ = session.Close()

		conn.touch()
		p.finishOperation(op, classifyExec(opCtx, waitErr))
	}()

	return op, nil
}

// execOutcome is the terminal state of a command.
type execOutcome struct {
	status     Status
	exitCode   int32
	exitSignal string
	message    string
}

// classifyExec turns "why did Wait return" into the status a plugin sees. The
// context cause is checked first: a killed command also reports a transport
// error, and the cancellation is the more useful answer.
func classifyExec(opCtx context.Context, waitErr error) execOutcome {
	if cause := context.Cause(opCtx); cause != nil {
		switch {
		case errors.Is(cause, errExecTimeout):
			return execOutcome{status: StatusTimedOut, exitCode: -1, message: errExecTimeout.Error()}
		case errors.Is(cause, errConnectionLost):
			return execOutcome{status: StatusFailed, exitCode: -1, message: errConnectionLost.Error()}
		}

		var canceled *cancelCauseError
		if errors.As(cause, &canceled) {
			return execOutcome{status: StatusCanceled, exitCode: -1, message: canceled.Error()}
		}

		return execOutcome{status: StatusCanceled, exitCode: -1, message: cause.Error()}
	}

	if waitErr == nil {
		return execOutcome{status: StatusCompleted, exitCode: 0}
	}

	var exitErr *ssh.ExitError
	if errors.As(waitErr, &exitErr) {
		if signal := exitErr.Signal(); signal != "" {
			return execOutcome{
				status:     StatusCompleted,
				exitCode:   -1,
				exitSignal: signal,
				message:    "killed by signal " + signal,
			}
		}

		return execOutcome{status: StatusCompleted, exitCode: int32(exitErr.ExitStatus())} //nolint:gosec
	}

	var missing *ssh.ExitMissingError
	if errors.As(waitErr, &missing) {
		return execOutcome{
			status:   StatusFailed,
			exitCode: -1,
			message:  "remote command exited without an exit status",
		}
	}

	return execOutcome{status: StatusFailed, exitCode: -1, message: waitErr.Error()}
}

// finishOperation publishes the outcome, retires the record and fires the
// completion callback if the plugin asked for one.
func (p *Sessions) finishOperation(op *operation, outcome execOutcome) {
	notify := op.finish(outcome)

	p.mu.Lock()
	if p.running > 0 {
		p.running--
	}
	p.finished = append(p.finished, op.id)
	evicted := p.evictOldFinishedLocked()
	p.mu.Unlock()

	for _, id := range evicted {
		p.dropOperation(id)
	}

	time.AfterFunc(p.svc.cfg.OperationRetention, func() { p.dropOperation(op.id) })

	p.svc.logger.Debug("plugin ssh command finished",
		slog.Uint64("plugin_id", p.pluginID),
		slog.String("operation_id", op.id),
		slog.String("status", string(outcome.status)),
		slog.Int("exit_code", int(outcome.exitCode)))

	if notify {
		p.svc.notifyCompleted(p, op)
	}
}

// evictOldFinishedLocked keeps the retained-output memory bounded, dropping
// the oldest finished operations first.
func (p *Sessions) evictOldFinishedLocked() []string {
	limit := p.svc.cfg.MaxRetainedOperations
	if len(p.finished) <= limit {
		return nil
	}

	evicted := make([]string, len(p.finished)-limit)
	copy(evicted, p.finished[:len(p.finished)-limit])
	p.finished = p.finished[len(p.finished)-limit:]

	return evicted
}

func (p *Sessions) dropOperation(operationID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.ops, operationID)

	for i, id := range p.finished {
		if id == operationID {
			p.finished = append(p.finished[:i], p.finished[i+1:]...)

			break
		}
	}
}

func (p *Sessions) operation(operationID string) (*operation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrSessionsClosed
	}

	op, ok := p.ops[operationID]
	if !ok {
		return nil, ErrOperationNotFound
	}

	return op, nil
}

// Cancel stops a running operation; it finishes as canceled.
func (p *Sessions) Cancel(operationID, reason string) error {
	op, err := p.operation(operationID)
	if err != nil {
		return err
	}

	if op.finishedNow() {
		return ErrOperationFinished
	}

	if reason == "" {
		reason = "canceled by plugin"
	}

	op.cancel(&cancelCauseError{reason: reason})

	return nil
}

// Snapshot reports an operation plus the captured output from the given
// offsets.
func (p *Sessions) Snapshot(operationID string, stdoutOffset, stderrOffset uint64) (ExecSnapshot, bool) {
	op, err := p.operation(operationID)
	if err != nil {
		return ExecSnapshot{}, false
	}

	return op.snapshot(stdoutOffset, stderrOffset), true
}

// WaitCompletion blocks until the operation finishes or the caller's budget
// runs out. A budget that expires is not an error: the command keeps running.
func (p *Sessions) WaitCompletion(ctx context.Context, operationID string) error {
	op, err := p.operation(operationID)
	if err != nil {
		return err
	}

	select {
	case <-op.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closedCh:
		return ErrSessionsClosed
	}
}

// SubscribeCompletion asks for the completion callback of an operation that is
// already running. A command that finished while the caller was deciding is
// replayed immediately, so the event is never lost.
func (p *Sessions) SubscribeCompletion(operationID string) error {
	op, err := p.operation(operationID)
	if err != nil {
		return err
	}

	if op.subscribe() {
		p.svc.notifyCompleted(p, op)
	}

	return nil
}

func (o *operation) finish(outcome execOutcome) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.status.Finished() {
		return false
	}

	o.status = outcome.status
	o.exitCode = outcome.exitCode
	o.exitSignal = outcome.exitSignal
	o.errMessage = outcome.message
	o.finishedAt = time.Now()

	close(o.done)

	return o.notify
}

func (o *operation) finishedNow() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	return o.status.Finished()
}

func (o *operation) cancel(cause error) {
	o.cancelFn(cause)
}

// subscribe records interest in the completion callback and reports whether it
// must be replayed because the operation already finished.
func (o *operation) subscribe() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.status.Finished() {
		return true
	}

	o.notify = true

	return false
}

func (o *operation) dropNotification() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.notify = false
}

func (o *operation) snapshot(stdoutOffset, stderrOffset uint64) ExecSnapshot {
	o.mu.Lock()
	snapshot := ExecSnapshot{
		OperationID: o.id,
		Handle:      o.handle,
		Status:      o.status,
		ExitCode:    o.exitCode,
		ExitSignal:  o.exitSignal,
		Error:       o.errMessage,
		StartedAt:   o.startedAt,
		FinishedAt:  o.finishedAt,
	}
	o.mu.Unlock()

	snapshot.Stdout, snapshot.StdoutNextOffset, snapshot.StdoutTruncated, snapshot.StdoutTotal =
		o.stdout.slice(stdoutOffset)
	snapshot.Stderr, snapshot.StderrNextOffset, snapshot.StderrTruncated, snapshot.StderrTotal =
		o.stderr.slice(stderrOffset)

	return snapshot
}
