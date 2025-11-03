package cache

import "time"

type Option func(o *Options)

type Options struct {
	Expiration time.Duration
}

func ApplyOptions(opts ...Option) *Options {
	o := &Options{}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

// WithExpiration allows to specify an expiration time when setting a value.
func WithExpiration(expiration time.Duration) Option {
	return func(o *Options) {
		o.Expiration = expiration
	}
}
