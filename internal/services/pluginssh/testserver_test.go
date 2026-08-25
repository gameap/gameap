package pluginssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	testPassword = "s3cret"

	// rejectExecCommand is answered with a failed exec request instead of
	// being run.
	rejectExecCommand = "@reject-exec"
)

// testSSHServer is a minimal in-process sshd: enough of the protocol to run
// the command vocabulary the tests need, so the engine is exercised against a
// real handshake, real channels and real exit statuses.
type testSSHServer struct {
	t        *testing.T
	listener net.Listener
	config   *ssh.ServerConfig

	hostKey     ssh.Signer
	fingerprint string

	mu         sync.Mutex
	acceptEnv  map[string]struct{}
	seenEnv    map[string]string
	closed     bool
	authorized ssh.PublicKey
}

func newTestSSHServer(t *testing.T, opts ...testServerOption) *testSSHServer {
	t.Helper()

	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(private)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &testSSHServer{
		t:           t,
		listener:    listener,
		hostKey:     signer,
		fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
		acceptEnv:   map[string]struct{}{"LANG": {}},
		seenEnv:     map[string]string{},
	}

	server.config = &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != testPassword {
				return nil, io.ErrUnexpectedEOF
			}

			return &ssh.Permissions{}, nil
		},
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			server.mu.Lock()
			authorized := server.authorized
			server.mu.Unlock()

			if authorized == nil || string(authorized.Marshal()) != string(key.Marshal()) {
				return nil, io.ErrUnexpectedEOF
			}

			return &ssh.Permissions{}, nil
		},
	}
	server.config.AddHostKey(signer)

	for _, opt := range opts {
		opt(server)
	}

	go server.acceptLoop()

	t.Cleanup(server.close)

	return server
}

func (s *testSSHServer) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()

		return
	}
	s.closed = true
	s.mu.Unlock()

	_ = s.listener.Close()
}

func (s *testSSHServer) addr() (host string, port uint32) {
	tcpAddr, ok := s.listener.Addr().(*net.TCPAddr)
	require.True(s.t, ok)

	return "127.0.0.1", uint32(tcpAddr.Port)
}

// testServerOption configures the server before it starts serving. Auth has to
// be settled at that point: the handshake goroutine reads ssh.ServerConfig by
// value, so changing it on a listening server races every incoming connection.
type testServerOption func(*testSSHServer)

// withKeyboardInteractive makes the server refuse plain password auth and
// answer with a challenge instead, the way a sshd with a PAM prompt does. The
// engine has to walk its keyboard-interactive fallback to get in.
func withKeyboardInteractive() testServerOption {
	return func(s *testSSHServer) {
		s.config.PasswordCallback = nil
		s.config.KeyboardInteractiveCallback = func(
			_ ssh.ConnMetadata,
			challenge ssh.KeyboardInteractiveChallenge,
		) (*ssh.Permissions, error) {
			answers, err := challenge("", "", []string{"Password: ", "Password again: "}, []bool{false, false})
			if err != nil {
				return nil, err
			}

			if len(answers) != 2 || answers[0] != testPassword || answers[1] != testPassword {
				return nil, io.ErrUnexpectedEOF
			}

			return &ssh.Permissions{}, nil
		}
	}
}

func (s *testSSHServer) authorizeKey(key ssh.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authorized = key
}

func (s *testSSHServer) allowEnv(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.acceptEnv[name] = struct{}{}
}

func (s *testSSHServer) envAllowed(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.acceptEnv[name]

	return ok
}

func (s *testSSHServer) recordEnv(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seenEnv[name] = value
}

func (s *testSSHServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		go s.serve(conn)
	}
}

func (s *testSSHServer) serve(conn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		_ = conn.Close()

		return
	}
	defer func() { _ = sshConn.Close() }()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")

			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			return
		}

		go s.handleSession(conn, channel, requests)
	}
}

// handleSession keeps consuming requests while a command runs, so a "signal"
// sent mid-execution reaches the command instead of queueing behind it. The
// channel is closed by whoever finishes last: the command goroutine after the
// exit status, or the loop when no command ever started.
func (s *testSSHServer) handleSession(conn net.Conn, channel ssh.Channel, requests <-chan *ssh.Request) {
	killed := make(chan struct{})
	var killOnce sync.Once
	execStarted := false

	for req := range requests {
		switch req.Type {
		case "env":
			var payload struct{ Name, Value string }
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)

				continue
			}

			allowed := s.envAllowed(payload.Name)
			if allowed {
				s.recordEnv(payload.Name, payload.Value)
			}
			_ = req.Reply(allowed, nil)
		case "signal":
			killOnce.Do(func() { close(killed) })
			_ = req.Reply(true, nil)
		case "exec":
			var payload struct{ Command string }
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				_ = req.Reply(false, nil)
				_ = channel.Close()

				return
			}
			// A server that refuses the exec request is what sshd does for a
			// command it will not run at all; nothing starts, so the engine
			// must report a start failure rather than an operation.
			if payload.Command == rejectExecCommand {
				_ = req.Reply(false, nil)

				continue
			}

			_ = req.Reply(true, nil)

			execStarted = true

			go func() {
				code := s.runCommand(conn, channel, payload.Command, killed)
				sendExitStatus(channel, code)
				_ = channel.Close()
			}()
		default:
			_ = req.Reply(false, nil)
		}
	}

	if !execStarted {
		_ = channel.Close()
	}
}

// runCommand is the tiny vocabulary the tests drive the server with.
func (s *testSSHServer) runCommand(
	conn net.Conn,
	channel ssh.Channel,
	command string,
	killed <-chan struct{},
) int {
	name, arg, _ := strings.Cut(command, " ")

	switch name {
	case "echo":
		_, _ = io.WriteString(channel, arg)

		return 0
	case "echo-stderr":
		_, _ = io.WriteString(channel.Stderr(), arg)

		return 0
	case "exit":
		code, _ := strconv.Atoi(arg)

		return code
	case "cat":
		_, _ = io.Copy(channel, channel)

		return 0
	case "spam":
		size, _ := strconv.Atoi(arg)
		_, _ = channel.Write(make([]byte, size))

		return 0
	case "env":
		s.mu.Lock()
		for key, value := range s.seenEnv {
			_, _ = io.WriteString(channel, key+"="+value+"\n")
		}
		s.mu.Unlock()

		return 0
	case "sleep":
		ms, _ := strconv.Atoi(arg)
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return 0
		case <-killed:
			return 137
		}
	case "die-signal":
		// The command died from a signal, so the server sends exit-signal and
		// no exit-status; the negative code keeps sendExitStatus quiet.
		sendExitSignal(channel, arg)

		return -1
	case "no-status":
		return -1
	case "kill-conn":
		_ = conn.Close()

		return -1
	default:
		_, _ = io.WriteString(channel.Stderr(), "unknown command: "+name)

		return 127
	}
}

// sendExitSignal reports that the command was killed rather than exited. A
// server sends this instead of an exit-status, and the engine must keep the
// two apart: a signalled command has no exit code to report.
func sendExitSignal(channel ssh.Channel, name string) {
	payload := ssh.Marshal(struct {
		Signal     string
		CoreDumped bool
		Error      string
		Lang       string
	}{Signal: name})

	_, _ = channel.SendRequest("exit-signal", false, payload)
}

// sendExitStatus reports the exit code; a negative code stands for "the server
// never sent one", which the engine must surface as a failure.
func sendExitStatus(channel ssh.Channel, code int) {
	if code < 0 {
		return
	}

	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(code))
	_, _ = channel.SendRequest("exit-status", false, payload)
}
