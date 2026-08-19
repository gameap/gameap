package enrollment

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/pkg/shellescape"
)

// SetupScriptOptions tune the generated installer the same way the query
// parameters of the admin-facing setup endpoint do.
type SetupScriptOptions struct {
	// Config is the daemon config path handed to gameapctl.
	Config string
	// GitHub builds the daemon from GitHub sources instead of a release.
	GitHub bool
	// Branch selects the source branch when GitHub is set.
	Branch string
}

const setupScriptTemplate = `#!/bin/bash
set -euo pipefail

CONNECT_URL=%s
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

// BuildSetupScript renders the bash installer that enrolls a daemon with the
// given connect URL. Everything interpolated into the script is shell-escaped:
// the URL embeds a host that may come from request headers or from a plugin,
// and the script runs as root on the target machine.
func BuildSetupScript(connectURL string, opts SetupScriptOptions) string {
	return fmt.Sprintf(setupScriptTemplate, shellescape.Quote(connectURL), BuildInstallCommand(opts))
}

// BuildInstallCommand is the gameapctl invocation the setup script ends with;
// it is also what an operator runs by hand on Windows.
func BuildInstallCommand(opts SetupScriptOptions) string {
	sb := strings.Builder{}

	sb.WriteString("\"$GAMEAPCTL_BIN\" daemon install --connect=\"$CONNECT_URL\"")

	if opts.Config != "" {
		sb.WriteString(" --config=")
		sb.WriteString(shellescape.Quote(opts.Config))
	}

	if opts.GitHub {
		sb.WriteString(" --github")
	}

	if opts.Branch != "" {
		sb.WriteString(" --branch=")
		sb.WriteString(shellescape.Quote(opts.Branch))
	}

	return sb.String()
}
