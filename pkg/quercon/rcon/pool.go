package rcon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/puddle/v2"
	"github.com/pkg/errors"
)

const (
	defaultPoolMaxSize = 3
	maxIdleTime        = 10 * time.Second
	cleanupInterval    = 5 * time.Second
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

type resourceWrapper struct {
	client     Client
	lastUsedAt time.Time
	mu         sync.Mutex
}

func (r *resourceWrapper) updateLastUsed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUsedAt = time.Now()
}

func (r *resourceWrapper) getLastUsed() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastUsedAt
}

type Pool struct {
	p           *puddle.Pool[*resourceWrapper]
	config      Config
	stopCleanup chan struct{}
	wg          sync.WaitGroup
}

func NewPool(config Config) (*Pool, error) {
	// Validate config before creating pool
	if _, err := NewClient(config); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	constructor := func(ctx context.Context) (*resourceWrapper, error) {
		client, err := NewClient(config)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create client")
		}

		if err := client.Open(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to open connection")
		}

		return &resourceWrapper{
			client:     client,
			lastUsedAt: time.Now(),
		}, nil
	}

	destructor := func(wrapper *resourceWrapper) {
		if wrapper != nil && wrapper.client != nil {
			if err := wrapper.client.Close(); err != nil {
				slog.Warn("Failed to close RCON connection", "error", err)
			}
		}
	}

	p, err := puddle.NewPool(&puddle.Config[*resourceWrapper]{
		Constructor: constructor,
		Destructor:  destructor,
		MaxSize:     defaultPoolMaxSize,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not create pool")
	}

	pool := &Pool{
		p:           p,
		config:      config,
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup goroutine
	pool.wg.Add(1)
	go pool.cleanupIdleConnections()

	return pool, nil
}

func (p *Pool) cleanupIdleConnections() {
	defer p.wg.Done()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			idleResources := p.p.AcquireAllIdle()
			now := time.Now()

			for _, res := range idleResources {
				if res.Value() != nil {
					lastUsed := res.Value().getLastUsed()
					if now.Sub(lastUsed) > maxIdleTime {
						// Destroy idle connection
						res.Destroy()
					} else {
						// Return to pool
						res.Release()
					}
				} else {
					res.Release()
				}
			}
		case <-p.stopCleanup:
			return
		}
	}
}

func (p *Pool) Close() {
	close(p.stopCleanup)
	p.wg.Wait()
	p.p.Close()
}

func (p *Pool) Acquire(ctx context.Context) (Client, error) {
	res, err := p.p.Acquire(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire connection from pool")
	}

	// Update last used time
	if res.Value() != nil {
		res.Value().updateLastUsed()
	}

	return &PooledClient{
		ctx: ctx,
		r:   res,
		p:   p.p,
	}, nil
}

func (p *Pool) TryAcquire(ctx context.Context) (Client, error) {
	res, err := p.p.TryAcquire(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not acquire connection from pool")
	}

	// Update last used time
	if res.Value() != nil {
		res.Value().updateLastUsed()
	}

	return &PooledClient{
		ctx: ctx,
		r:   res,
		p:   p.p,
	}, nil
}

func (p *Pool) Stat() *puddle.Stat {
	return p.p.Stat()
}

type PooledClient struct {
	ctx context.Context
	r   *puddle.Resource[*resourceWrapper]
	p   *puddle.Pool[*resourceWrapper]
}

func (c *PooledClient) Open(_ context.Context) error {
	// Connection is already opened and authenticated during pool construction
	return nil
}

func (c *PooledClient) Close() (err error) {
	if c.r == nil || c.r.Value() == nil {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			err = newPuddlePanicError(r)
		}
	}()

	// Update last used time before releasing
	c.r.Value().updateLastUsed()
	c.r.Release()
	c.r = nil

	return nil
}

func (c *PooledClient) Execute(ctx context.Context, command string) (string, error) {
	if c.r == nil || c.r.Value() == nil {
		return "", errors.New("connection not established")
	}

	wrapper := c.r.Value()
	result, err := wrapper.client.Execute(ctx, command)
	if err != nil {
		// On error, destroy the connection so a new one will be created
		c.r.Destroy()

		return "", errors.Wrap(err, "failed to execute command")
	}

	// Update last used time after successful execution
	wrapper.updateLastUsed()

	return result, nil
}
