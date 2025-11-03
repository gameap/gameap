package daemon

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type marshaler interface {
	MarshalBINN() ([]byte, error)
}

type fileRaw []byte

// testContext creates a test context.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	return ctx
}

// MockDaemonServer is a mock TLS server that implements the GameAP daemon protocol.
type MockDaemonServer struct {
	listener    net.Listener
	tlsConfig   *tls.Config
	addr        string
	wg          sync.WaitGroup
	done        chan struct{}
	t           *testing.T
	mu          sync.Mutex
	connections []net.Conn

	// Captured Requests for inspection
	Requests      []any
	ReceivedFiles [][]byte

	// Pre-prepared responses to write in sequence
	Responses []any
}

// NewMockDaemonServer creates a new mock daemon server.
func NewMockDaemonServer(t *testing.T) (*MockDaemonServer, error) {
	t.Helper()

	// Load server certificate and key
	serverCert, err := tls.X509KeyPair([]byte(daemonServerCert), []byte(daemonServerKey))
	if err != nil {
		return nil, fmt.Errorf("failed to load server key pair: %w", err)
	}

	// Create CA cert pool from daemon server cert (which is also the CA)
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(daemonServerCert)) {
		return nil, errors.New("failed to add CA cert to pool")
	}

	// Configure TLS with mutual authentication
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	// Listen on random available port
	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	t.Logf("Mock daemon server started at %s", listener.Addr())

	server := &MockDaemonServer{
		listener:    listener,
		tlsConfig:   tlsConfig,
		addr:        listener.Addr().String(),
		done:        make(chan struct{}),
		t:           t,
		connections: make([]net.Conn, 0),
	}

	// Set up default prepared responses using legacy fields
	server.Responses = []any{
		&binnapi.StatusVersionResponseMessage{
			Version:   "3.9.0",
			BuildDate: "2024-01-01",
		},
		&binnapi.StatusInfoBaseResponseMessage{
			Uptime:        "1h23m",
			WorkingTasks:  "5",
			WaitingTasks:  "2",
			OnlineServers: "4",
		},
	}

	return server, nil
}

// Start begins accepting connections.
func (s *MockDaemonServer) Start() {
	s.wg.Go(func() {
		s.acceptConnections()
	})

	// Wait for server to be ready by attempting to connect
	s.waitForReady()
}

// waitForReady polls the server until it's ready to accept connections.
func (s *MockDaemonServer) waitForReady() {
	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			s.t.Fatal("mock daemon server failed to start within timeout")

			return
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", s.addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()

				return
			}
		}
	}
}

// Stop stops the server and closes all connections.
func (s *MockDaemonServer) Stop() {
	// Check if already stopped
	select {
	case <-s.done:
		return
	default:
	}

	close(s.done)
	err := s.listener.Close()
	if err != nil {
		s.t.Logf("Failed to close listener: %v", err)
	}

	s.mu.Lock()
	for _, conn := range s.connections {
		err := conn.Close()
		if err != nil {
			s.t.Logf("Failed to close connection: %v", err)
		}
	}
	s.connections = nil
	s.mu.Unlock()

	s.wg.Wait()
}

// Addr returns the server's address.
func (s *MockDaemonServer) Addr() string {
	return s.addr
}

// AssertRequestCount asserts that the server received exactly n requests.
func (s *MockDaemonServer) AssertRequestCount(expected int) {
	s.t.Helper()
	s.mu.Lock()
	actual := len(s.Requests)
	s.mu.Unlock()
	require.Equal(s.t, expected, actual, "expected %d requests, got %d", expected, actual)
}

// AssertMinRequestCount asserts that the server received at least n requests.
func (s *MockDaemonServer) AssertMinRequestCount(value int) {
	s.t.Helper()
	s.mu.Lock()
	actual := len(s.Requests)
	s.mu.Unlock()
	require.GreaterOrEqual(s.t, actual, value, "expected at least %d requests, got %d", value, actual)
}

// GetRequestBytes returns the raw request bytes at the specified index.
func (s *MockDaemonServer) GetRequestBytes(index int) []byte {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.Requests) {
		s.t.Fatalf("request index %d out of bounds (total requests: %d)", index, len(s.Requests))

		return nil
	}

	if b, ok := s.Requests[index].(anyMessage); ok {
		return b
	}

	s.t.Fatalf("request at index %d is not anyMessage type", index)

	return nil
}

// AssertRequestEquals compares the request at the given index with the expected message.
// The expected message must implement MarshalBINN() method.
func (s *MockDaemonServer) AssertRequestEquals(index int, expected marshaler) {
	s.t.Helper()

	// Get the actual request bytes
	actualBytes := s.GetRequestBytes(index)

	// Marshal the expected message
	expectedBytes, err := expected.MarshalBINN()
	require.NoError(s.t, err, "failed to marshal expected message")

	// Compare bytes
	require.Equal(s.t, expectedBytes, actualBytes,
		"request at index %d does not match expected message", index)
}

// AssertAnyRequestEquals checks if any of the captured requests matches the expected message.
// Returns true if a match is found, otherwise fails the test.
func (s *MockDaemonServer) AssertAnyRequestEquals(expected marshaler) {
	s.t.Helper()

	// Marshal the expected message
	expectedBytes, err := expected.MarshalBINN()
	require.NoError(s.t, err, "failed to marshal expected message")

	s.mu.Lock()
	requestCount := len(s.Requests)
	s.mu.Unlock()

	// Check each request for a match
	for i := range requestCount {
		actualBytes := s.GetRequestBytes(i)
		if string(expectedBytes) == string(actualBytes) {
			// Match found
			return
		}
	}

	// No match found - fail the test
	require.Fail(s.t, "expected message not found in any captured request",
		"Expected message not found in any of the %d captured requests", requestCount)
}

// UnmarshalRequest unmarshals the request at the given index into the provided message.
// The message must implement UnmarshalBINN() method.
func (s *MockDaemonServer) UnmarshalRequest(index int, message interface {
	UnmarshalBINN([]byte) error
}) {
	s.t.Helper()

	// Get the request bytes
	requestBytes := s.GetRequestBytes(index)

	// Unmarshal into the provided message
	err := message.UnmarshalBINN(requestBytes)
	require.NoError(s.t, err, "failed to unmarshal request at index %d", index)
}

func (s *MockDaemonServer) AssertReceivedFileEquals(expected []byte) {
	s.mu.Lock()
	files := make([][]byte, len(s.ReceivedFiles))
	copy(files, s.ReceivedFiles)
	s.mu.Unlock()

	for _, file := range files {
		if bytes.Equal(file, expected) {
			return
		}
	}

	s.t.Fatal("expected file not found in received files")
}

// Host returns the server's host.
func (s *MockDaemonServer) Host() string {
	host, _, _ := net.SplitHostPort(s.addr)

	return host
}

// Port returns the server's port.
func (s *MockDaemonServer) Port() int {
	_, portStr, _ := net.SplitHostPort(s.addr)
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	return port
}

// acceptConnections accepts incoming connections.
func (s *MockDaemonServer) acceptConnections() {
	for {
		select {
		case <-s.done:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.t.Logf("Accept error: %v", err)

				continue
			}
		}

		s.mu.Lock()
		s.connections = append(s.connections, conn)
		s.mu.Unlock()

		s.wg.Go(func() {
			s.handleConnection(conn)
		})
	}
}

// handleConnection handles a single connection.
func (s *MockDaemonServer) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		for i, c := range s.connections {
			if c == conn {
				s.connections = append(s.connections[:i], s.connections[i+1:]...)

				break
			}
		}
		s.mu.Unlock()
	}()

	// Set read deadline
	err := conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err != nil {
		s.t.Logf("Failed to set read deadline: %v", err)

		return
	}

	// Read login message
	var loginMessage binnapi.LoginRequestMessage

	err = binnapi.ReadMessage(conn, &loginMessage)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return
		}

		s.t.Logf("Failed to read login message: %v", err)

		return
	}

	// Send login response (success)
	loginResponse := &binnapi.BaseResponseMessage{
		Code: binnapi.StatusCodeOK,
		Info: "OK",
	}
	err = binnapi.WriteMessage(conn, loginResponse)
	if err != nil {
		s.t.Logf("Failed to write login response: %v", err)

		return
	}

	// Handle Requests using pre-prepared responses
	s.handlePreparedResponses(conn)
}

// handlePreparedResponses writes pre-prepared responses in sequence.
func (s *MockDaemonServer) handlePreparedResponses(conn net.Conn) {
	responseIndex := 0

	for {
		select {
		case <-s.done:
			return
		default:
		}

		// Reset read deadline
		err := conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if err != nil {
			s.t.Logf("Failed to set read deadline: %v", err)

			return
		}

		// Read the request to consume it from the connection
		var request anyMessage
		err = binnapi.ReadMessage(conn, &request)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}

			s.t.Logf("Failed to read request: %v", err)

			return
		}

		s.mu.Lock()
		s.Requests = append(s.Requests, request)
		s.mu.Unlock()

		// Check if we have more responses to send
		if responseIndex >= len(s.Responses) {
			s.t.Logf("No more pre-prepared responses, closing connection")

			return
		}

		// Write the next pre-prepared response
		response := s.Responses[responseIndex]
		responseIndex++

		// Type assert to ensure the response can be marshaled
		// All valid response types should already implement the necessary interface
		err = binnapi.WriteMessage(conn, response.(marshaler))
		if err != nil {
			s.t.Logf("Failed to write pre-prepared response: %v", err)

			return
		}

		s.t.Logf("Wrote pre-prepared response %d/%d", responseIndex, len(s.Responses))

		// If next response is fileRaw, send it as is it
		if responseIndex < len(s.Responses) {
			if raw, ok := s.Responses[responseIndex].(fileRaw); ok {
				// Write raw bytes directly
				_, err = conn.Write(raw)
				if err != nil {
					s.t.Logf("Failed to write raw file response: %v", err)

					return
				}

				s.t.Logf("Wrote raw file response %d/%d", responseIndex+1, len(s.Responses))

				responseIndex++
			}
		}

		uploadMsg := &binnapi.UploadRequestMessage{}
		err = uploadMsg.UnmarshalBINN(request)
		if err != nil {
			// Do nothing if not an upload request
			continue
		}

		// For upload requests, read the file data
		b := make([]byte, uploadMsg.FileSize)
		n, err := conn.Read(b)
		if err != nil {
			s.t.Logf("Failed to read uploaded file data: %v", err)

			return
		}

		s.mu.Lock()
		s.ReceivedFiles = append(s.ReceivedFiles, b[:n])
		s.mu.Unlock()

		s.t.Logf("Read uploaded file data: expected %d bytes, got %d bytes", uploadMsg.FileSize, n)

		// Send the next response after receiving the uploaded file
		if responseIndex < len(s.Responses) {
			response := s.Responses[responseIndex]
			responseIndex++

			err = binnapi.WriteMessage(conn, response.(marshaler))
			if err != nil {
				s.t.Logf("Failed to write upload completion response: %v", err)

				return
			}

			s.t.Logf("Wrote upload completion response %d/%d", responseIndex, len(s.Responses))
		}
	}
}

// TestMockDaemonServerWithPreparedResponses tests the mock daemon server with pre-prepared responses.
func TestMockDaemonServerWithPreparedResponses(t *testing.T) {
	// Create and start mock server
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	// Configure pre-prepared responses
	mockServer.Responses = []any{
		&binnapi.StatusVersionResponseMessage{
			Version:   "4.0.0-custom",
			BuildDate: "2025-12-31",
		},
		&binnapi.StatusInfoBaseResponseMessage{
			Uptime:        "2h45m",
			WorkingTasks:  "7",
			WaitingTasks:  "3",
			OnlineServers: "5",
		},
	}

	mockServer.Start()

	t.Logf("Mock server started at %s", mockServer.Addr())

	// Create a pool to connect to the mock server
	pool, err := NewPool(config{
		Host:              mockServer.Host(),
		Port:              mockServer.Port(),
		ServerCertificate: []byte(daemonServerCert),
		ClientCertificate: []byte(clientCert),
		PrivateKey:        []byte(clientKey),
		Timeout:           10 * time.Second,
		Mode:              binnapi.ModeStatus,
	})
	require.NoError(t, err)

	// Acquire connection
	conn, err := pool.Acquire(testContext(t))
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	// Request version - should get first pre-prepared response
	err = binnapi.WriteMessage(conn, binnapi.StatusRequestVersion)
	require.NoError(t, err)

	var versionResp binnapi.StatusVersionResponseMessage
	err = binnapi.ReadMessage(conn, &versionResp)
	require.NoError(t, err)

	require.Equal(t, "4.0.0-custom", versionResp.Version)
	require.Equal(t, "2025-12-31", versionResp.BuildDate)

	t.Logf("Version: %s, BuildDate: %s", versionResp.Version, versionResp.BuildDate)

	// Request base status - should get second pre-prepared response
	err = binnapi.WriteMessage(conn, binnapi.StatusRequestStatusBase)
	require.NoError(t, err)

	var baseResp binnapi.StatusInfoBaseResponseMessage
	err = binnapi.ReadMessage(conn, &baseResp)
	require.NoError(t, err)

	require.Equal(t, "2h45m", baseResp.Uptime)
	require.Equal(t, "7", baseResp.WorkingTasks)
	require.Equal(t, "3", baseResp.WaitingTasks)
	require.Equal(t, "5", baseResp.OnlineServers)

	t.Logf("Uptime: %s, WorkingTasks: %s, WaitingTasks: %s, OnlineServers: %s",
		baseResp.Uptime,
		baseResp.WorkingTasks,
		baseResp.WaitingTasks,
		baseResp.OnlineServers,
	)
}

// TestMockDaemonServer tests the mock daemon server.
func TestMockDaemonServer(t *testing.T) {
	// Create and start mock server
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Start()

	t.Logf("Mock server started at %s", mockServer.Addr())

	// Create a pool to connect to the mock server
	pool, err := NewPool(config{
		Host:              mockServer.Host(),
		Port:              mockServer.Port(),
		ServerCertificate: []byte(daemonServerCert),
		ClientCertificate: []byte(clientCert),
		PrivateKey:        []byte(clientKey),
		Timeout:           10 * time.Second,
		Mode:              binnapi.ModeStatus,
	})
	require.NoError(t, err)

	// Acquire connection
	conn, err := pool.Acquire(testContext(t))
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	// Request version
	err = binnapi.WriteMessage(conn, binnapi.StatusRequestVersion)
	require.NoError(t, err)

	var versionResp binnapi.StatusVersionResponseMessage
	err = binnapi.ReadMessage(conn, &versionResp)
	require.NoError(t, err)

	t.Logf("Version: %s, BuildDate: %s", versionResp.Version, versionResp.BuildDate)

	// Request base status
	err = binnapi.WriteMessage(conn, binnapi.StatusRequestStatusBase)
	require.NoError(t, err)

	var baseResp binnapi.StatusInfoBaseResponseMessage
	err = binnapi.ReadMessage(conn, &baseResp)
	require.NoError(t, err)
}

// TestMockDaemonServerRequestAssertion tests the request assertion functions.
func TestMockDaemonServerRequestAssertion(t *testing.T) {
	// Create and start mock server
	mockServer, err := NewMockDaemonServer(t)
	require.NoError(t, err)
	defer mockServer.Stop()

	mockServer.Start()

	// Create a pool to connect to the mock server
	pool, err := NewPool(config{
		Host:              mockServer.Host(),
		Port:              mockServer.Port(),
		ServerCertificate: []byte(daemonServerCert),
		ClientCertificate: []byte(clientCert),
		PrivateKey:        []byte(clientKey),
		Timeout:           10 * time.Second,
		Mode:              binnapi.ModeStatus,
	})
	require.NoError(t, err)

	// Acquire connection
	conn, err := pool.Acquire(testContext(t))
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	// Send version request
	err = binnapi.WriteMessage(conn, binnapi.StatusRequestVersion)
	require.NoError(t, err)

	var versionResp binnapi.StatusVersionResponseMessage
	err = binnapi.ReadMessage(conn, &versionResp)
	require.NoError(t, err)

	// Send status base request
	err = binnapi.WriteMessage(conn, binnapi.StatusRequestStatusBase)
	require.NoError(t, err)

	var baseResp binnapi.StatusInfoBaseResponseMessage
	err = binnapi.ReadMessage(conn, &baseResp)
	require.NoError(t, err)

	// Wait briefly to ensure all requests are captured
	time.Sleep(100 * time.Millisecond)

	// Assert request count
	mockServer.AssertRequestCount(2)

	// Assert first request equals StatusRequestVersion
	mockServer.AssertRequestEquals(0, binnapi.StatusRequestVersion)

	// Assert second request equals StatusRequestStatusBase
	mockServer.AssertRequestEquals(1, binnapi.StatusRequestStatusBase)

	// Demonstrate UnmarshalRequest - unmarshal and inspect the raw bytes
	var req1 anyMessage
	mockServer.UnmarshalRequest(0, &req1)
	t.Logf("First request raw bytes length: %d", len(req1))

	// Demonstrate GetRequestBytes
	req2Bytes := mockServer.GetRequestBytes(1)
	t.Logf("Second request raw bytes length: %d", len(req2Bytes))

	// Demonstrate AssertAnyRequestEquals - verify that StatusRequestVersion is in the captured requests
	mockServer.AssertAnyRequestEquals(binnapi.StatusRequestVersion)

	// Demonstrate AssertAnyRequestEquals - verify that StatusRequestStatusBase is in the captured requests
	mockServer.AssertAnyRequestEquals(binnapi.StatusRequestStatusBase)
}

type anyMessage []byte

func (msg *anyMessage) UnmarshalBINN(b []byte) error {
	*msg = b

	return nil
}
