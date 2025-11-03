package daemon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/pkg/errors"
)

const (
	defaultTimeout = 10 * time.Second
)

var (
	ErrServerCertificateInvalid = errors.New("failed to append server certificate to pool")
	ErrConnectionNotEstablished = errors.New("connection is not established")
)

type config struct {
	Host              string
	Port              int
	Username          string
	Password          string
	ServerCertificate []byte // Server CA certificate (PEM encoded)
	ClientCertificate []byte // Client certificate (PEM encoded)
	PrivateKey        []byte // Private key (PEM encoded)
	PrivateKeyPass    string // Private key passphrase
	Timeout           time.Duration
	Mode              binnapi.Mode
}

type Connection struct {
	conn net.Conn

	cfg        config
	maxBufSize int
}

func Connect(ctx context.Context, cfg config) (*Connection, error) {
	if cfg.Port == 0 {
		cfg.Port = 31717
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Mode == 0 {
		cfg.Mode = binnapi.ModeNoAuth
	}

	c := &Connection{
		cfg:        cfg,
		maxBufSize: 10 * 1024 * 1024, // 10 MB
	}

	if err := c.connect(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Connection) connect(ctx context.Context) error {
	// Load server CA certificate
	serverCertPool := x509.NewCertPool()
	if len(c.cfg.ServerCertificate) > 0 {
		if !serverCertPool.AppendCertsFromPEM(c.cfg.ServerCertificate) {
			return ErrServerCertificateInvalid
		}
	}

	// Load client certificate and private key
	var certificates []tls.Certificate
	if len(c.cfg.ClientCertificate) > 0 && len(c.cfg.PrivateKey) > 0 {
		// If private key is encrypted, decrypt it
		// Note: Go's tls.X509KeyPair doesn't support encrypted keys directly
		// For encrypted keys, you'd need to decrypt them first using x509.DecryptPEMBlock (deprecated)
		// or use x509.ParsePKCS8PrivateKey with appropriate decryption
		cert, err := tls.X509KeyPair(c.cfg.ClientCertificate, c.cfg.PrivateKey)
		if err != nil {
			return errors.Wrap(err, "failed to load client certificate and key")
		}

		certificates = append(certificates, cert)
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		RootCAs:            serverCertPool,
		Certificates:       certificates,
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         tls.VersionTLS10,
		MaxVersion:         tls.VersionTLS13,
	}

	// Connect to the daemon
	address := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	dialer := &net.Dialer{
		Timeout: c.cfg.Timeout,
	}

	rawConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return errors.Wrapf(err, "could not connect to host: %s, port: %d",
			c.cfg.Host, c.cfg.Port)
	}

	conn := tls.Client(rawConn, tlsConfig)
	err = conn.HandshakeContext(ctx)
	if err != nil {
		return errors.Wrapf(err, "could not connect to host: %s, port: %d",
			c.cfg.Host, c.cfg.Port)
	}

	// Set connection deadlines
	if err := conn.SetDeadline(time.Now().Add(c.cfg.Timeout)); err != nil {
		closeErr := conn.Close()
		if closeErr != nil {
			slog.Warn("could not set connection deadline", "error", closeErr)
		}

		return errors.WithMessage(err, "could not set connection deadline")
	}

	c.conn = conn

	if err := binnapi.Login(ctx, conn, c.cfg.Mode, c.cfg.Username, c.cfg.Password); err != nil {
		closeErr := conn.Close()
		if closeErr != nil {
			slog.Warn("could not close connection", "error", closeErr)
		}

		return err
	}

	return nil
}

func (c *Connection) Write(buffer []byte) (int, error) {
	if c.conn == nil {
		return 0, ErrConnectionNotEstablished
	}

	n, err := c.conn.Write(buffer)
	if err != nil {
		return n, errors.Wrap(err, "socket write failed")
	}

	return n, nil
}

func (c *Connection) Read(b []byte) (n int, err error) {
	if c.conn == nil {
		return 0, ErrConnectionNotEstablished
	}

	return c.conn.Read(b)
}

func (c *Connection) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil

		return err
	}

	return nil
}

func (c *Connection) LocalAddr() net.Addr {
	if c.conn == nil {
		return nil
	}

	return c.conn.LocalAddr()
}

func (c *Connection) RemoteAddr() net.Addr {
	if c.conn == nil {
		return nil
	}

	return c.conn.RemoteAddr()
}

func (c *Connection) SetDeadline(t time.Time) error {
	if c.conn == nil {
		return ErrConnectionNotEstablished
	}

	return c.conn.SetDeadline(t)
}

func (c *Connection) SetReadDeadline(t time.Time) error {
	if c.conn == nil {
		return ErrConnectionNotEstablished
	}

	return c.conn.SetReadDeadline(t)
}

func (c *Connection) SetWriteDeadline(t time.Time) error {
	if c.conn == nil {
		return ErrConnectionNotEstablished
	}

	return c.conn.SetWriteDeadline(t)
}
