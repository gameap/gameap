package rcon

import (
	"bytes"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// header is the four-byte out-of-band marker shared by the id Software
	// connectionless packet formats (GoldSource, Quake 2, Quake 3).
	header = "\xff\xff\xff\xff"
	// maxResponseSize is the read buffer for a single UDP datagram. Game servers can send RCON
	// replies up to the network MTU, so this is sized to the maximum UDP payload to avoid
	// truncating (and silently dropping) the tail of a datagram.
	maxResponseSize = 65535
	// responseIdleTimeout bounds the wait for follow-up datagrams of a multi-packet response.
	// Servers send every part of a large RCON reply back-to-back right after the command,
	// so a short idle gap means the response is complete.
	responseIdleTimeout = 500 * time.Millisecond
	// maxReassembledResponseSize caps the total size of a reassembled multi-packet response.
	// Without it a server streaming datagrams faster than the idle timeout would grow the
	// reassembly buffer without bound. Sized well above real-world replies (a full cvarlist
	// dump is tens of KiB).
	maxReassembledResponseSize = 1 << 20 // 1 MiB
)

// datagramBody extracts the usable payload of one response datagram. Returning nil means the
// datagram carries nothing this protocol can use — a foreign, truncated or malformed packet —
// and the reassembly loop skips it without failing the whole response.
type datagramBody func(datagram []byte) []byte

// readDatagram reads a single UDP datagram whole. The raw bytes are returned as received;
// stripping any protocol framing is the caller's job.
func readDatagram(conn net.Conn, timeout time.Duration) ([]byte, error) {
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
	}

	buffer := make([]byte, maxResponseSize)

	n, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}

	return buffer[:n], nil
}

// readReassembledResponse collects a response that a server may split across several datagrams.
// The first datagram gets the full timeout; the rest are read until the socket goes idle, which
// is the only end-of-response signal these protocols provide.
//
// A timeout error can therefore only originate from the first read: later timeouts end the loop
// normally. Callers that treat server silence as an empty response rely on that.
func readReassembledResponse(conn net.Conn, timeout time.Duration, body datagramBody) (string, error) {
	part, err := readDatagram(conn, timeout)
	if err != nil {
		return "", err
	}

	buffer := bytes.Buffer{}
	buffer.Write(body(part))

	for {
		part, err = readDatagram(conn, responseIdleTimeout)
		if err != nil {
			if isTimeoutError(err) {
				break
			}

			return "", err
		}

		buffer.Write(body(part))

		// Stop once the accumulated response exceeds the cap; any datagrams still
		// queued afterwards are drained before the next command.
		if buffer.Len() > maxReassembledResponseSize {
			break
		}
	}

	return strings.TrimSpace(buffer.String()), nil
}

// drainStaleDatagrams discards datagrams already sitting in the socket receive buffer
// (late replies to previous commands) so the next read starts from a clean slate.
func drainStaleDatagrams(conn net.Conn) {
	buffer := make([]byte, maxResponseSize)

	for {
		_ = conn.SetReadDeadline(time.Now())

		if _, err := conn.Read(buffer); err != nil {
			return
		}
	}
}

func isTimeoutError(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}
