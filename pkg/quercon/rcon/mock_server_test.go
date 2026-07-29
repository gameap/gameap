package rcon

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

const mockServerReadTimeout = 5 * time.Second

// scriptedTCPServer accepts incoming TCP connections and hands each one to handler in a goroutine.
// Tests script per-connection behaviour by closing over channels / atomic counters in handler.
type scriptedTCPServer struct {
	listener net.Listener
	addr     string

	wg     sync.WaitGroup
	closed chan struct{}
}

func newScriptedTCPServer(t *testing.T, handler func(net.Conn)) *scriptedTCPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("scriptedTCPServer: listen: %v", err)
	}

	srv := &scriptedTCPServer{
		listener: listener,
		addr:     listener.Addr().String(),
		closed:   make(chan struct{}),
	}

	srv.wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-srv.closed:
					return
				default:
					if errors.Is(err, net.ErrClosed) {
						return
					}

					return
				}
			}
			srv.wg.Go(func() {
				defer func() { _ = conn.Close() }()
				handler(conn)
			})
		}
	})

	t.Cleanup(srv.Close)

	return srv
}

func (s *scriptedTCPServer) Close() {
	select {
	case <-s.closed:
		return
	default:
		close(s.closed)
	}
	_ = s.listener.Close()
	s.wg.Wait()
}

// scriptedUDPServer reads one datagram at a time and answers via handler. handler returns the
// response datagrams (each is written as a separate UDP packet); nil or empty means "do not reply".
// writeDelay optionally paces the response datagrams of a multi-packet reply.
type scriptedUDPServer struct {
	conn       net.PacketConn
	addr       string
	writeDelay time.Duration

	wg     sync.WaitGroup
	closed chan struct{}
}

func newScriptedUDPServer(t *testing.T, handler func(req []byte, idx int) [][]byte) *scriptedUDPServer {
	t.Helper()

	return newPacedScriptedUDPServer(t, 0, handler)
}

func newPacedScriptedUDPServer(
	t *testing.T,
	writeDelay time.Duration,
	handler func(req []byte, idx int) [][]byte,
) *scriptedUDPServer {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("scriptedUDPServer: listen: %v", err)
	}

	srv := &scriptedUDPServer{
		conn:       pc,
		addr:       pc.LocalAddr().String(),
		writeDelay: writeDelay,
		closed:     make(chan struct{}),
	}

	srv.wg.Go(func() {
		buf := make([]byte, 4096)
		idx := 0
		for {
			_ = pc.SetReadDeadline(time.Now().Add(mockServerReadTimeout))
			n, peer, err := pc.ReadFrom(buf)
			if err != nil {
				select {
				case <-srv.closed:
					return
				default:
					if errors.Is(err, net.ErrClosed) {
						return
					}
					var nerr net.Error
					if errors.As(err, &nerr) && nerr.Timeout() {
						return
					}

					return
				}
			}
			req := make([]byte, n)
			copy(req, buf[:n])

			resp := handler(req, idx)
			idx++
			for _, dgram := range resp {
				_, _ = pc.WriteTo(dgram, peer)
				// Pacing keeps a burst of response datagrams from overrunning the client's
				// kernel receive buffer faster than the client can drain it.
				if srv.writeDelay > 0 {
					time.Sleep(srv.writeDelay)
				}
			}
		}
	})

	t.Cleanup(srv.Close)

	return srv
}

func (s *scriptedUDPServer) Close() {
	select {
	case <-s.closed:
		return
	default:
		close(s.closed)
	}
	_ = s.conn.Close()
	s.wg.Wait()
}
