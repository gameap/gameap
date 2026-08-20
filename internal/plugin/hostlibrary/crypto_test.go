package hostlibrary

import (
	"context"
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/sdk/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCryptoService_RandomUint64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		max       uint64
		wantError string
	}{
		{
			name: "valid_max_100",
			max:  100,
		},
		{
			name: "valid_max_1",
			max:  1,
		},
		{
			name: "valid_max_uint64",
			max:  ^uint64(0),
		},
		{
			name:      "zero_max",
			max:       0,
			wantError: "max must be greater than 0",
		},
	}

	svc := NewCryptoService()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.RandomUint64(ctx, &crypto.RandomUint64Request{Max: tt.max})
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Less(t, resp.Value, tt.max)
		})
	}
}

func TestCryptoService_RandomUint64_Distribution(t *testing.T) {
	t.Parallel()
	svc := NewCryptoService()
	ctx := context.Background()

	counts := make(map[uint64]int)
	iterations := 1000
	maxVal := uint64(10)

	for range iterations {
		resp, err := svc.RandomUint64(ctx, &crypto.RandomUint64Request{Max: maxVal})
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		counts[resp.Value]++
	}

	for i := range maxVal {
		assert.Greater(t, counts[i], 0, "value %d should appear at least once", i)
	}
}

func TestCryptoService_RandomString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		length    int32
		charset   *string
		wantError string
	}{
		{
			name:   "default_charset",
			length: 16,
		},
		{
			name:    "custom_charset",
			length:  8,
			charset: new("abc"),
		},
		{
			name:    "hex_charset",
			length:  32,
			charset: new("0123456789abcdef"),
		},
		{
			name:    "empty_charset_uses_default",
			length:  10,
			charset: new(""),
		},
		{
			name:      "zero_length",
			length:    0,
			wantError: "length must be greater than 0",
		},
		{
			name:      "negative_length",
			length:    -1,
			wantError: "length must be greater than 0",
		},
		{
			name:      "exceeds_max_length",
			length:    1024*1024 + 1,
			wantError: "length exceeds maximum",
		},
	}

	svc := NewCryptoService()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.RandomString(ctx, &crypto.RandomStringRequest{
				Length:  tt.length,
				Charset: tt.charset,
			})
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Len(t, resp.Value, int(tt.length))

			if tt.charset != nil && *tt.charset != "" {
				for _, c := range resp.Value {
					assert.True(t, strings.ContainsRune(*tt.charset, c),
						"character %c not in charset %s", c, *tt.charset)
				}
			}
		})
	}
}

func TestCryptoService_RandomString_Uniqueness(t *testing.T) {
	t.Parallel()
	svc := NewCryptoService()
	ctx := context.Background()

	results := make(map[string]bool)
	iterations := 100

	for range iterations {
		resp, err := svc.RandomString(ctx, &crypto.RandomStringRequest{Length: 32})
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		assert.False(t, results[resp.Value], "generated duplicate string")
		results[resp.Value] = true
	}
}

func TestCryptoService_Argon2Hash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		password  string
		params    *crypto.Argon2Params
		wantError string
	}{
		{
			name:     "default_params",
			password: "mysecretpassword",
		},
		{
			name:     "custom_params",
			password: "password123",
			params: &crypto.Argon2Params{
				Memory:      32768,
				Time:        3,
				Parallelism: 2,
				SaltLength:  32,
				KeyLength:   64,
			},
		},
		{
			name:     "unicode_password",
			password: "пароль密码🔐",
		},
		{
			name:      "empty_password",
			password:  "",
			wantError: "password cannot be empty",
		},
		{
			name:     "memory_too_low",
			password: "test",
			params: &crypto.Argon2Params{
				Memory: 512,
			},
			wantError: "memory must be at least",
		},
		{
			name:     "memory_too_high",
			password: "test",
			params: &crypto.Argon2Params{
				Memory: 5000000,
			},
			wantError: "memory exceeds maximum",
		},
		{
			name:     "salt_too_short",
			password: "test",
			params: &crypto.Argon2Params{
				SaltLength: 4,
			},
			wantError: "salt length must be at least 8",
		},
		{
			name:     "key_too_short",
			password: "test",
			params: &crypto.Argon2Params{
				KeyLength: 8,
			},
			wantError: "key length must be at least 16",
		},
	}

	svc := NewCryptoService()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.Argon2Hash(ctx, &crypto.Argon2HashRequest{
				Password: tt.password,
				Params:   tt.params,
			})
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.True(t, strings.HasPrefix(resp.Hash, "$argon2id$v=19$"))
		})
	}
}

func TestCryptoService_Argon2Hash_UniqueHashes(t *testing.T) {
	t.Parallel()
	svc := NewCryptoService()
	ctx := context.Background()

	password := "samepassword"
	results := make(map[string]bool)

	for range 10 {
		resp, err := svc.Argon2Hash(ctx, &crypto.Argon2HashRequest{
			Password: password,
		})
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		assert.False(t, results[resp.Hash], "same password should produce different hashes")
		results[resp.Hash] = true
	}
}

func TestCryptoService_Argon2Verify(t *testing.T) {
	t.Parallel()
	svc := NewCryptoService()
	ctx := context.Background()

	hashResp, err := svc.Argon2Hash(ctx, &crypto.Argon2HashRequest{
		Password: "testpassword",
	})
	require.NoError(t, err)
	require.Nil(t, hashResp.Error)
	validHash := hashResp.Hash

	tests := []struct {
		name      string
		password  string
		hash      string
		wantMatch bool
		wantError string
	}{
		{
			name:      "correct_password",
			password:  "testpassword",
			hash:      validHash,
			wantMatch: true,
		},
		{
			name:      "wrong_password",
			password:  "wrongpassword",
			hash:      validHash,
			wantMatch: false,
		},
		{
			name:      "empty_password",
			password:  "",
			hash:      validHash,
			wantMatch: false,
			wantError: "password cannot be empty",
		},
		{
			name:      "empty_hash",
			password:  "testpassword",
			hash:      "",
			wantMatch: false,
			wantError: "hash cannot be empty",
		},
		{
			name:      "invalid_hash_format",
			password:  "testpassword",
			hash:      "notahash",
			wantMatch: false,
			wantError: "invalid hash format",
		},
		{
			name:      "wrong_algorithm",
			password:  "testpassword",
			hash:      "$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA",
			wantMatch: false,
			wantError: "unsupported algorithm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.Argon2Verify(ctx, &crypto.Argon2VerifyRequest{
				Password: tt.password,
				Hash:     tt.hash,
			})
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.wantMatch, resp.Match)
		})
	}
}

func TestCryptoService_Argon2Verify_CustomParams(t *testing.T) {
	t.Parallel()
	svc := NewCryptoService()
	ctx := context.Background()

	customParams := &crypto.Argon2Params{
		Memory:      32768,
		Time:        3,
		Parallelism: 2,
		SaltLength:  24,
		KeyLength:   48,
	}

	hashResp, err := svc.Argon2Hash(ctx, &crypto.Argon2HashRequest{
		Password: "custompassword",
		Params:   customParams,
	})
	require.NoError(t, err)
	require.Nil(t, hashResp.Error)

	verifyResp, err := svc.Argon2Verify(ctx, &crypto.Argon2VerifyRequest{
		Password: "custompassword",
		Hash:     hashResp.Hash,
	})
	require.NoError(t, err)
	require.Nil(t, verifyResp.Error)
	assert.True(t, verifyResp.Match)

	wrongResp, err := svc.Argon2Verify(ctx, &crypto.Argon2VerifyRequest{
		Password: "wrongpassword",
		Hash:     hashResp.Hash,
	})
	require.NoError(t, err)
	require.Nil(t, wrongResp.Error)
	assert.False(t, wrongResp.Match)
}

func TestCryptoHostLibrary_New(t *testing.T) {
	t.Parallel()
	lib := NewCryptoHostLibrary()
	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
