package testcontainer

import (
	"database/sql"

	"github.com/gameap/gameap/internal/config"
)

// Container holds dependencies for tests.
type Container struct {
	config *config.Config
	db     *sql.DB
}

type Option func(*Container)

func WithConfig(cfg *config.Config) Option {
	return func(c *Container) {
		c.config = cfg
	}
}

func WithEmptyConfig() Option {
	return func(c *Container) {
		c.config = &config.Config{}
	}
}

func WithDB(db *sql.DB) Option {
	return func(c *Container) {
		c.db = db
	}
}

func NewContainer(opts ...Option) *Container {
	c := &Container{}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Container) Config() *config.Config {
	return c.config
}

func (c *Container) DB() *sql.DB {
	return c.db
}
