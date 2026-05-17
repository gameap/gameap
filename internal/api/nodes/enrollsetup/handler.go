package enrollsetup

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/shellescape"
	"github.com/pkg/errors"
)

type Handler struct {
	enrollmentSvc *enrollment.Service
	responder     base.Responder
	grpcExtHost   string
	grpcPort      uint16
	grpcExtPort   uint16
	panelHost     string
}

func NewHandler(
	enrollmentSvc *enrollment.Service,
	responder base.Responder,
	panelHost string,
	grpcExtHost string,
	grpcPort uint16,
	grpcExtPort uint16,
) *Handler {
	return &Handler{
		enrollmentSvc: enrollmentSvc,
		responder:     responder,
		panelHost:     panelHost,
		grpcExtHost:   grpcExtHost,
		grpcPort:      grpcPort,
		grpcExtPort:   grpcExtPort,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.enrollmentSvc == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("enrollment is not available, gRPC is disabled"),
			http.StatusServiceUnavailable,
		))

		return
	}

	key, err := api.NewInputReader(r).ReadString("key")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid key"),
			http.StatusBadRequest,
		))

		return
	}

	if err := h.enrollmentSvc.SetupKeyManager().Validate(ctx, key); err != nil {
		if errors.Is(err, enrollment.ErrInvalidSetupKey) || errors.Is(err, enrollment.ErrSetupKeyNotConfigured) {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				errors.WithMessage(err, "invalid setup key"),
				http.StatusForbidden,
			))

			return
		}

		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to validate setup key"))

		return
	}

	grpcHost := h.resolveGRPCHost(r)
	grpcPort := h.grpcPort
	if h.grpcExtPort > 0 {
		grpcPort = h.grpcExtPort
	}

	connectURL := enrollment.FormatConnectURL(grpcHost, grpcPort, key)

	queryReader := api.NewQueryReader(r)
	config, _ := queryReader.ReadString("config")
	github, _ := queryReader.ReadString("github")
	branch, _ := queryReader.ReadString("branch")

	script := h.buildSetupScript(connectURL, config, github == "true", branch)

	rw.Header().Set("Content-Type", "text/plain")
	_, _ = rw.Write([]byte(script))
}

func (h *Handler) resolveGRPCHost(r *http.Request) string {
	if h.grpcExtHost != "" {
		return h.grpcExtHost
	}

	host := h.panelHost
	if host == "" {
		host = r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
	}

	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	if idx := strings.IndexByte(host, ':'); idx != -1 {
		host = host[:idx]
	}

	return host
}

const setupScriptTemplate = `#!/bin/bash
set -euo pipefail

CONNECT_URL="%s"
GAMEAPCTL_BIN=""

_tmpfile=""
_tmpbin=""
cleanup() {
  [[ -n "${_tmpfile}" && -f "${_tmpfile}" ]] && rm -f "${_tmpfile}"
  [[ -n "${_tmpbin}"  && -f "${_tmpbin}"  ]] && rm -f "${_tmpbin}"
  return 0
}
trap cleanup EXIT

if [[ "$(id -u)" -ne 0 ]]; then
  echo "This script must be run as root." >&2
  echo "Process substitution (bash <(curl ...)) does not survive sudo." >&2
  echo "Try:" >&2
  echo "  curl -fsSL '<setup-link>' -o gameap-setup.sh && sudo bash gameap-setup.sh <args>" >&2
  exit 1
fi

for cmd in curl tar install; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: '$cmd' is required but not installed." >&2
    echo "Install it via your package manager and re-run this script." >&2
    exit 1
  fi
done

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  i?86)          ARCH="386" ;;
  arm*)          ARCH="arm" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

if command -v gameapctl >/dev/null 2>&1; then
  echo "gameapctl found, running self-update..."
  gameapctl self-update || true
  GAMEAPCTL_BIN="$(command -v gameapctl)"
else
  echo "Downloading gameapctl..."
  VERSION=$(curl -sL https://api.github.com/repos/gameap/gameapctl/releases \
            | grep -m1 '"tag_name"' \
            | sed 's/.*"tag_name": *"//;s/".*//' || true)
  if [[ -z "${VERSION}" ]]; then
    echo "Failed to detect latest gameapctl version" >&2
    exit 1
  fi
  ARCHIVE="gameapctl-${VERSION}-${OS}-${ARCH}.tar.gz"
  DOWNLOAD_URL="https://github.com/gameap/gameapctl/releases/download/${VERSION}/${ARCHIVE}"
  echo "Downloading ${DOWNLOAD_URL}"
  _tmpfile="$(mktemp -t gameapctl.XXXXXX.tar.gz)"
  curl -sLf -o "${_tmpfile}" "${DOWNLOAD_URL}"
  _tmpbin="$(mktemp -t gameapctl.XXXXXX)"
  tar -xzOf "${_tmpfile}" gameapctl > "${_tmpbin}"
  install -m 0755 "${_tmpbin}" /usr/local/bin/gameapctl
  GAMEAPCTL_BIN="/usr/local/bin/gameapctl"
fi

case ":${PATH}:" in
  *:/usr/local/bin:*) ;;
  *) export PATH="/usr/local/bin:${PATH}" ;;
esac
hash -r

%s "$@"
`

func (h *Handler) buildSetupScript(connectURL, config string, github bool, branch string) string {
	return fmt.Sprintf(setupScriptTemplate, connectURL, h.buildInstallCmd(config, github, branch))
}

func (h *Handler) buildInstallCmd(
	config string, github bool, branch string,
) string {
	sb := strings.Builder{}

	sb.WriteString("\"$GAMEAPCTL_BIN\" daemon install --connect=\"$CONNECT_URL\"")

	if config != "" {
		sb.WriteString(" --config=")
		sb.WriteString(shellescape.Quote(config))
	}

	if github {
		sb.WriteString(" --github")
	}

	if branch != "" {
		sb.WriteString(" --branch=")
		sb.WriteString(shellescape.Quote(branch))
	}

	return sb.String()
}
