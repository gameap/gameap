package rcon

import (
	"bytes"
	"context"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// quakeResponseKeyword is the out-of-band command every id Tech 2/3 server uses to send console
// output back: the reply is header + "print\n" + chunk.
const quakeResponseKeyword = "print"

var (
	// ErrRconDisabled is returned when the server answers that no RCON password is configured,
	// so remote console is switched off rather than merely rejecting our credentials.
	ErrRconDisabled = errors.New("rcon is not enabled on the server")

	// ErrPasswordContainsWhitespace rejects a password the id Tech protocols cannot express.
	// The server locates the command by walking to the first space after the password, so a
	// password containing whitespace would push part of itself into the command position and
	// execute something the caller never asked for.
	ErrPasswordContainsWhitespace = errors.New("rcon password must not contain whitespace")
)

// quakeSpec captures the only thing that differs between the id Tech 2 and id Tech 3 RCON
// dialects: the console text a server prints when it refuses the request. Both engines put
// Com_BeginRedirect before the password check, so these messages do reach the client.
type quakeSpec struct {
	authRefused  string
	rconDisabled string
}

var (
	quake3Spec = quakeSpec{
		authRefused:  "Bad rconpassword.",
		rconDisabled: "No rconpassword set on the server.",
	}
	// Quake 2 reports an unset password with the same message as a wrong one, so it has no
	// distinct rcon-disabled marker.
	quake2Spec = quakeSpec{authRefused: "Bad rcon_password."}
)

// Quake implements RCON for the id Tech 2 and id Tech 3 engine families: Quake 2, Quake 3
// Arena, Quake Live, Return to Castle Wolfenstein, Wolfenstein: Enemy Territory, the Call of
// Duty series and the ioquake3 derivatives.
//
// The wire format is a single connectionless UDP datagram in each direction; there is no
// handshake and no session, so Open only dials. Large replies arrive as several datagrams
// because the server flushes its redirect buffer in ~1 KiB chunks.
type Quake struct {
	address    string
	password   string
	timeout    time.Duration
	connection net.Conn
	spec       quakeSpec
}

// NewQuake3 creates a client for id Tech 3 servers.
func NewQuake3(config Config) (*Quake, error) {
	return newQuake(config, quake3Spec)
}

// NewQuake2 creates a client for id Tech 2 servers.
func NewQuake2(config Config) (*Quake, error) {
	return newQuake(config, quake2Spec)
}

func newQuake(config Config, spec quakeSpec) (*Quake, error) {
	adapter := &Quake{
		address:  config.Address,
		password: config.Password,
		timeout:  config.Timeout,
		spec:     spec,
	}

	return adapter, nil
}

// Open dials the server. The protocol is stateless and carries the password on every command,
// so there is nothing to authenticate here — a wrong password surfaces from Execute. Probing
// with a throw-away command is deliberately avoided: vanilla Quake 3 throttles RCON to one
// packet per 500 ms server-wide and would silently drop the real command that follows.
func (q *Quake) Open(ctx context.Context) error {
	if strings.ContainsAny(q.password, " \t\r\n") {
		return ErrPasswordContainsWhitespace
	}

	dialer := &net.Dialer{
		Timeout: q.timeout,
	}

	conn, err := dialer.DialContext(ctx, "udp", q.address)
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}

	q.connection = conn

	return nil
}

func (q *Quake) Close() error {
	if q.connection != nil {
		err := q.connection.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (q *Quake) Execute(_ context.Context, command string) (string, error) {
	drainStaleDatagrams(q.connection)

	cmd := header + "rcon " + q.password + " " + command

	if _, err := q.connection.Write([]byte(cmd)); err != nil {
		return "", err
	}

	response, err := readReassembledResponse(q.connection, q.timeout, quakeDatagramBody)
	if err != nil {
		return "", err
	}

	if err := q.checkRefusal(response); err != nil {
		return "", err
	}

	return response, nil
}

// checkRefusal turns the server's refusal text into a sentinel error. The engines report both
// conditions as ordinary console output, so matching the message is the only way to tell a
// rejected password from a command that legitimately printed nothing.
func (q *Quake) checkRefusal(response string) error {
	if q.spec.rconDisabled != "" && strings.HasPrefix(response, q.spec.rconDisabled) {
		return errors.WithMessage(ErrRconDisabled, q.spec.rconDisabled)
	}

	if strings.HasPrefix(response, q.spec.authRefused) {
		return errors.WithMessage(ErrAuthenticationFailed, q.spec.authRefused)
	}

	return nil
}

// quakeDatagramBody strips the out-of-band header and the "print" keyword. Foreign datagrams
// (status responses, other clients' traffic reaching a shared socket) yield nil and are skipped.
// Chunks are concatenated without a separator because the server's ~1 KiB flush cuts mid-line.
func quakeDatagramBody(datagram []byte) []byte {
	if !bytes.HasPrefix(datagram, []byte(header)) {
		return nil
	}

	body := datagram[len(header):]
	if !bytes.HasPrefix(body, []byte(quakeResponseKeyword)) {
		return nil
	}

	body = body[len(quakeResponseKeyword):]

	// Vanilla servers send "print\n"; some Call of Duty builds omit the newline.
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}

	return body
}
