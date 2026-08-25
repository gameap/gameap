package enrollment

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
)

const ConnectScheme = "grpc"

var ErrInvalidConnectURL = errors.New("invalid connect URL")

type ConnectInfo struct {
	Host     string
	Port     uint16
	SetupKey string
}

// FormatConnectURL builds the URL a daemon is handed. net.JoinHostPort brackets
// a bare IPv6 literal, which url.Parse needs to tell the address from the port.
func FormatConnectURL(host string, port uint16, setupKey string) string {
	return fmt.Sprintf("%s://%s/%s",
		ConnectScheme, net.JoinHostPort(host, strconv.Itoa(int(port))), setupKey)
}

func ParseConnectURL(rawURL string) (*ConnectInfo, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse connect URL")
	}

	if u.Scheme != ConnectScheme {
		return nil, errors.WithMessage(ErrInvalidConnectURL, fmt.Sprintf("scheme %q, expected %q", u.Scheme, ConnectScheme))
	}

	host := u.Hostname()
	if host == "" {
		return nil, errors.New("host is required")
	}

	portStr := u.Port()
	if portStr == "" {
		return nil, errors.New("port is required")
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, errors.WithMessage(err, "invalid port")
	}

	key := u.Path
	if len(key) > 0 && key[0] == '/' {
		key = key[1:]
	}
	if key == "" {
		return nil, errors.New("setup key is required")
	}

	return &ConnectInfo{
		Host:     host,
		Port:     uint16(port),
		SetupKey: key,
	}, nil
}
