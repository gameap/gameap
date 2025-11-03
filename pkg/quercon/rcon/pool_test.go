package rcon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPool(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid_source_config",
			config: Config{
				Address:  "127.0.0.1:27015",
				Password: "test",
				Protocol: ProtocolSource,
				Timeout:  5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid_goldsource_config",
			config: Config{
				Address:  "127.0.0.1:27015",
				Password: "test",
				Protocol: ProtocolGoldSrc,
				Timeout:  5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid_protocol",
			config: Config{
				Address:  "127.0.0.1:27015",
				Password: "test",
				Protocol: "invalid",
				Timeout:  5 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPool(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, pool)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pool)
				defer pool.Close()
				assert.NotNil(t, pool.p)
			}
		})
	}
}

func TestPool_Close(t *testing.T) {
	config := Config{
		Address:  "127.0.0.1:27015",
		Password: "test",
		Protocol: ProtocolSource,
		Timeout:  5 * time.Second,
	}

	pool, err := NewPool(config)
	require.NoError(t, err)
	require.NotNil(t, pool)

	pool.Close()

	// After close, pool should not allow new acquisitions
	ctx := context.Background()
	_, err = pool.TryAcquire(ctx)
	assert.Error(t, err)
}

func TestPool_Stat(t *testing.T) {
	config := Config{
		Address:  "127.0.0.1:27015",
		Password: "test",
		Protocol: ProtocolSource,
		Timeout:  5 * time.Second,
	}

	pool, err := NewPool(config)
	require.NoError(t, err)
	require.NotNil(t, pool)
	defer pool.Close()

	stat := pool.Stat()
	require.NotNil(t, stat)

	// Initially pool should be empty
	assert.Equal(t, int32(0), stat.AcquiredResources())
	assert.Equal(t, int32(0), stat.TotalResources())
}

func TestPooledClient_Open(t *testing.T) {
	config := Config{
		Address:  "127.0.0.1:27015",
		Password: "test",
		Protocol: ProtocolSource,
		Timeout:  5 * time.Second,
	}

	pool, err := NewPool(config)
	require.NoError(t, err)
	require.NotNil(t, pool)
	defer pool.Close()

	// Open should always succeed for pooled clients (already authenticated)
	client := &PooledClient{}
	err = client.Open(context.Background())
	assert.NoError(t, err)
}

func TestPooledClient_Close(t *testing.T) {
	tests := []struct {
		name    string
		client  *PooledClient
		wantErr bool
	}{
		{
			name:    "nil_resource",
			client:  &PooledClient{r: nil},
			wantErr: false,
		},
		{
			name: "close_twice",
			client: &PooledClient{
				r: nil, // Already released
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.Close()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPooledClient_Execute_NilResource(t *testing.T) {
	client := &PooledClient{r: nil}

	_, err := client.Execute(context.Background(), "status")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not established")
}

func TestPuddlePanicError(t *testing.T) {
	details := "test panic details"
	err := newPuddlePanicError(details)

	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "panic in puddle")
	assert.Contains(t, err.Error(), details)

	// Check type using errors.As
	var panicErr puddlePanicError
	require.True(t, errors.As(err, &panicErr))
	assert.Equal(t, details, panicErr.details)
}
