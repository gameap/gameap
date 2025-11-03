package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/jackc/puddle/v2"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
)

const (
	defaultPoolMaxSize = 3

	retryAttempts = 3
	retryDelay    = 50 * time.Millisecond
)

type puddlePanicError struct {
	details any
}

func newPuddlePanicError(details any) error {
	return puddlePanicError{details: details}
}

func (e puddlePanicError) Error() string {
	return fmt.Sprintf("panic in puddle: %v", e.details)
}

type Pool struct {
	p *puddle.Pool[net.Conn]
}

func NewPool(cfg config) (*Pool, error) {
	constructor := func(ctx context.Context) (net.Conn, error) {
		return Connect(ctx, cfg)
	}

	destructor := func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			slog.Warn("Failed to close connection", "error", err)
		}
	}

	p, err := puddle.NewPool(&puddle.Config[net.Conn]{
		Constructor: constructor,
		Destructor:  destructor,
		MaxSize:     defaultPoolMaxSize,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create pool")
	}

	return &Pool{
		p: p,
	}, nil
}

func (p *Pool) Close() error {
	p.p.Close()

	return nil
}

func (p *Pool) Acquire(ctx context.Context) (net.Conn, error) {
	var res *puddle.Resource[net.Conn]
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		res, err = p.p.Acquire(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		if res.IdleDuration() < defaultTimeout {
			break
		}

		slog.DebugContext(
			ctx, "reconnecting idle connection",
			slog.Duration("idle_duration", res.IdleDuration()),
		)

		res.Destroy()
	}

	return &PooledConn{
		ctx: ctx, // store context for potential reconnection
		r:   res,
		p:   p.p,
	}, nil
}

func (p *Pool) TryAcquire(ctx context.Context) (net.Conn, error) {
	res, err := p.p.TryAcquire(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire connection from pool")
	}

	return &PooledConn{
		ctx: ctx, // store context for potential reconnection
		r:   res,
		p:   p.p,
	}, nil
}

func (p *Pool) Stat() *puddle.Stat {
	return p.p.Stat()
}

func (p *Pool) WriteContext(ctx context.Context, buffer []byte) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = newPuddlePanicError(r)
		}
	}()

	return p.writeContext(ctx, buffer)
}

func (p *Pool) writeContext(ctx context.Context, buffer []byte) (int, error) {
	res, err := p.p.Acquire(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "could not acquire connection from pool")
	}
	defer res.Release()

	var n int
	err = Retry(retryAttempts, retryDelay, func() error {
		n, err = res.Value().Write(buffer)
		if err != nil {
			res.Destroy()

			var acqErr error
			res, acqErr = p.p.Acquire(ctx)
			if acqErr != nil {
				return multierr.Append(err, errors.Wrap(acqErr, "could not acquire connection from pool"))
			}
		}

		return err
	})

	if err != nil {
		return n, errors.Wrap(err, "could not write to connection")
	}

	return n, nil
}

func Retry(attempts int, delay time.Duration, fn func() error) error {
	if attempts < 1 {
		return errors.New("attempts must be at least 1")
	}

	var err error
	for i := range attempts {
		err = fn()
		if err == nil {
			return nil
		}

		// Don't sleep after the last attempt
		if i < attempts-1 {
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("after %d attempts, last error: %w", attempts, err)
}

type PooledConn struct {
	mu  sync.Mutex
	ctx context.Context
	r   *puddle.Resource[net.Conn]
	p   *puddle.Pool[net.Conn]
}

func (c *PooledConn) Read(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return 0, errors.New("connection not established")
	}

	return c.r.Value().Read(b)
}

func (c *PooledConn) Write(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return 0, errors.New("connection not established")
	}

	err = Retry(retryAttempts, retryDelay, func() error {
		n, err = c.r.Value().Write(b)
		if err != nil {
			slog.Debug(
				"could not write to connection",
				slog.String("error", err.Error()),
			)

			c.r.Destroy()

			var acqErr error
			c.r, acqErr = c.p.Acquire(c.ctx)
			if acqErr != nil {
				return multierr.Append(err, errors.Wrap(acqErr, "could not acquire connection from pool"))
			}
		}

		return err
	})

	return n, err
}

func (c *PooledConn) Close() (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			err = newPuddlePanicError(r)
		}
	}()

	c.r.Release()

	c.r = nil

	return nil
}

func (c *PooledConn) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return nil
	}

	return c.r.Value().LocalAddr()
}

func (c *PooledConn) RemoteAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return nil
	}

	return c.r.Value().RemoteAddr()
}

func (c *PooledConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return errors.New("connection not established")
	}

	return c.r.Value().SetDeadline(t)
}

func (c *PooledConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return errors.New("connection not established")
	}

	return c.r.Value().SetReadDeadline(t)
}

func (c *PooledConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.r == nil || c.r.Value() == nil {
		return errors.New("connection not established")
	}

	return c.r.Value().SetWriteDeadline(t)
}
