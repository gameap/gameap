package hostlibrary

import (
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/services/pluginssh"
	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/gameap/gameap/pkg/shellescape"
	"github.com/pkg/errors"
)

var (
	errPathRequired    = errors.New("path is required")
	errInvalidFileMode = errors.New(
		"invalid file mode: the value is octal permission bits up to 0o777 (write 0o644, not decimal 644); " +
			"setuid/setgid/sticky bits are not supported")
)

func keyTypeFromProto(keyType sshsdk.KeyType) pluginssh.KeyType {
	switch keyType {
	case sshsdk.KeyType_KEY_TYPE_RSA_4096:
		return pluginssh.KeyTypeRSA4096
	case sshsdk.KeyType_KEY_TYPE_ECDSA_P256:
		return pluginssh.KeyTypeECDSAP256
	case sshsdk.KeyType_KEY_TYPE_UNSPECIFIED, sshsdk.KeyType_KEY_TYPE_ED25519:
		fallthrough
	default:
		return pluginssh.KeyTypeED25519
	}
}

func connectParamsFromProto(req *sshsdk.ConnectRequest) pluginssh.ConnectParams {
	params := pluginssh.ConnectParams{
		Host:           req.Host,
		Port:           req.Port,
		User:           req.User,
		ConnectTimeout: time.Duration(req.ConnectTimeoutMs) * time.Millisecond,
		IdleTimeout:    time.Duration(req.IdleTimeoutMs) * time.Millisecond,
	}

	if req.Auth != nil {
		params.Password = req.Auth.Password
		params.PrivateKeyPEM = req.Auth.PrivateKeyPem
		params.Passphrase = req.Auth.Passphrase
	}

	if req.HostKey != nil {
		params.HostKey = pluginssh.HostKeyPolicy{
			AcceptAny:          req.HostKey.AcceptAny,
			FingerprintsSHA256: req.HostKey.FingerprintsSha256,
			PublicKeys:         req.HostKey.PublicKeys,
		}
	}

	return params
}

func execParamsFromProto(req *sshsdk.ExecRequest) pluginssh.ExecParams {
	return pluginssh.ExecParams{
		Handle:         req.Handle,
		Command:        req.Command,
		Stdin:          req.Stdin,
		Env:            req.Env,
		Timeout:        time.Duration(req.TimeoutSeconds) * time.Second,
		MaxOutputBytes: int(req.MaxOutputBytes),
	}
}

func execStatusToProto(status pluginssh.Status) sshsdk.ExecStatus {
	switch status {
	case pluginssh.StatusRunning:
		return sshsdk.ExecStatus_EXEC_STATUS_RUNNING
	case pluginssh.StatusCompleted:
		return sshsdk.ExecStatus_EXEC_STATUS_COMPLETED
	case pluginssh.StatusFailed:
		return sshsdk.ExecStatus_EXEC_STATUS_FAILED
	case pluginssh.StatusTimedOut:
		return sshsdk.ExecStatus_EXEC_STATUS_TIMED_OUT
	case pluginssh.StatusCanceled:
		return sshsdk.ExecStatus_EXEC_STATUS_CANCELED
	default:
		return sshsdk.ExecStatus_EXEC_STATUS_UNSPECIFIED
	}
}

func execSyncResponseFromSnapshot(snapshot pluginssh.ExecSnapshot) *sshsdk.ExecSyncResponse {
	response := &sshsdk.ExecSyncResponse{
		Success:         true,
		OperationId:     snapshot.OperationID,
		Completed:       true,
		OpSuccess:       snapshot.Succeeded(),
		Status:          execStatusToProto(snapshot.Status),
		ExitCode:        snapshot.ExitCode,
		ExitSignal:      snapshot.ExitSignal,
		Stdout:          snapshot.Stdout,
		Stderr:          snapshot.Stderr,
		StdoutTruncated: snapshot.StdoutTruncated,
		StderrTruncated: snapshot.StderrTruncated,
		StartedAt:       unixMilli(snapshot.StartedAt),
		FinishedAt:      unixMilli(snapshot.FinishedAt),
	}

	if snapshot.Error != "" {
		response.OpError = new(snapshot.Error)
	}

	return response
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}

	return t.UnixMilli()
}

// remoteFailureMessage explains a non-zero exit in the terms a plugin author
// debugs in: the status, and the exit code when there is one.
func remoteFailureMessage(snapshot pluginssh.ExecSnapshot) string {
	if snapshot.Error != "" {
		return snapshot.Error
	}

	if snapshot.Status == pluginssh.StatusCompleted {
		return "remote command exited with code " + strconv.FormatInt(int64(snapshot.ExitCode), 10)
	}

	return "remote command " + string(snapshot.Status)
}

const writeFileTempSuffix = ".gameap-tmp"

// writeFileCommand builds a shell pipeline that writes stdin to a temporary
// file next to the target and renames it into place. The temp file is created
// under umask 077 and chmod-ed before the rename, so the content is never
// readable wider than the requested mode, an interrupted transfer leaves no
// partial target, and an existing symlink is replaced instead of followed.
// Quoting keeps hostile names one argument; the -- marker stops a leading dash
// from being read as an option.
func writeFileCommand(path string, mode uint32) (string, error) {
	if mode > 0o777 {
		return "", errInvalidFileMode
	}

	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errPathRequired
	}

	quoted := shellescape.Quote(trimmed)
	temp := shellescape.Quote(trimmed + writeFileTempSuffix)

	command := "{ cat > " + temp
	if mode > 0 {
		command = "umask 077; " + command
		command += " && chmod " + strconv.FormatUint(uint64(mode), 8) + " -- " + temp
	}
	command += " && mv -f -- " + temp + " " + quoted + "; }"
	command += " || { rc=$?; rm -f -- " + temp + "; exit $rc; }"

	return command, nil
}

func readFileCommand(path string) (string, error) {
	quoted, err := quoteRemotePath(path)
	if err != nil {
		return "", err
	}

	return "cat -- " + quoted, nil
}

func quoteRemotePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errPathRequired
	}

	return shellescape.Quote(trimmed), nil
}
