package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite

	cacheInstance Cache
	fn            func(t *testing.T) Cache
}

func NewCacheSuite(fn func(t *testing.T) Cache) *Suite {
	return &Suite{
		fn: fn,
	}
}

func (s *Suite) SetupTest() {
	s.cacheInstance = s.fn(s.T())

	err := s.cacheInstance.Clear(context.Background())
	require.NoError(s.T(), err)
}

func (s *Suite) TearDownTest() {
	if s.cacheInstance != nil {
		_ = s.cacheInstance.Clear(context.Background())
	}
}

func (s *Suite) TestGetNotFound() {
	ctx := context.Background()

	value, err := s.cacheInstance.Get(ctx, "non_existent_key")
	assert.ErrorIs(s.T(), err, ErrNotFound)
	assert.Nil(s.T(), value)
}

func (s *Suite) TestSetAndGetSimple() {
	ctx := context.Background()

	testCases := []struct {
		name  string
		key   string
		value any
	}{
		{
			name:  "string_value",
			key:   "test_key_string",
			value: "test_value",
		},
		{
			name:  "int_value",
			key:   "test_key_int",
			value: 42,
		},
		{
			name:  "float_value",
			key:   "test_key_float",
			value: 3.14,
		},
		{
			name:  "bool_value",
			key:   "test_key_bool",
			value: true,
		},
		{
			name:  "map_value",
			key:   "test_key_map",
			value: map[string]any{"foo": "bar", "count": 123},
		},
		{
			name:  "slice_value",
			key:   "test_key_slice",
			value: []string{"one", "two", "three"},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			err := s.cacheInstance.Set(ctx, tc.key, tc.value)
			require.NoError(t, err)

			retrievedValue, err := s.cacheInstance.Get(ctx, tc.key)
			require.NoError(t, err)

			// Type assertion based on expected type
			switch expected := tc.value.(type) {
			case string:
				assert.Equal(t, expected, retrievedValue)
			case int:
				// JSON unmarshals numbers as float64, but in-memory cache preserves type
				if floatVal, ok := retrievedValue.(float64); ok {
					assert.Equal(t, float64(expected), floatVal)
				} else {
					assert.Equal(t, expected, retrievedValue)
				}
			case float64:
				assert.Equal(t, expected, retrievedValue)
			case bool:
				assert.Equal(t, expected, retrievedValue)
			case map[string]any:
				resultMap, ok := retrievedValue.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, expected["foo"], resultMap["foo"])
				// Handle both int (in-memory) and float64 (JSON) for count
				if floatCount, ok := resultMap["count"].(float64); ok {
					assert.Equal(t, float64(expected["count"].(int)), floatCount)
				} else {
					assert.Equal(t, expected["count"], resultMap["count"])
				}
			case []string:
				// Handle both []string (in-memory) and []any (JSON)
				if resultSlice, ok := retrievedValue.([]any); ok {
					require.Len(t, resultSlice, len(expected))
					for i, v := range expected {
						assert.Equal(t, v, resultSlice[i])
					}
				} else if resultSlice, ok := retrievedValue.([]string); ok {
					assert.Equal(t, expected, resultSlice)
				} else {
					t.Errorf("unexpected type for slice: %T", retrievedValue)
				}
			}
		})
	}
}

func (s *Suite) TestSetWithExpiration() {
	ctx := context.Background()

	expiration := 1 * time.Second
	err := s.cacheInstance.Set(ctx, "expiring_key", "expiring_value", WithExpiration(expiration))
	require.NoError(s.T(), err)

	value, err := s.cacheInstance.Get(ctx, "expiring_key")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "expiring_value", value)

	time.Sleep(expiration + 500*time.Millisecond)

	value, err = s.cacheInstance.Get(ctx, "expiring_key")
	assert.ErrorIs(s.T(), err, ErrNotFound)
	assert.Nil(s.T(), value)
}

func (s *Suite) TestSetOverwrite() {
	ctx := context.Background()

	err := s.cacheInstance.Set(ctx, "overwrite_key", "initial_value")
	require.NoError(s.T(), err)

	value, err := s.cacheInstance.Get(ctx, "overwrite_key")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "initial_value", value)

	err = s.cacheInstance.Set(ctx, "overwrite_key", "updated_value")
	require.NoError(s.T(), err)

	value, err = s.cacheInstance.Get(ctx, "overwrite_key")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "updated_value", value)
}

func (s *Suite) TestDelete() {
	ctx := context.Background()

	err := s.cacheInstance.Set(ctx, "delete_key", "delete_value")
	require.NoError(s.T(), err)

	value, err := s.cacheInstance.Get(ctx, "delete_key")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "delete_value", value)

	err = s.cacheInstance.Delete(ctx, "delete_key")
	require.NoError(s.T(), err)

	value, err = s.cacheInstance.Get(ctx, "delete_key")
	assert.ErrorIs(s.T(), err, ErrNotFound)
	assert.Nil(s.T(), value)
}

func (s *Suite) TestDeleteNonExistentKey() {
	ctx := context.Background()

	err := s.cacheInstance.Delete(ctx, "non_existent_key")
	assert.NoError(s.T(), err)
}

func (s *Suite) TestPullReturnsValueAndRemoves() {
	ctx := context.Background()

	require.NoError(s.T(), s.cacheInstance.Set(ctx, "pull_key", "pull_value"))

	value, err := s.cacheInstance.Pull(ctx, "pull_key")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "pull_value", value)

	value, err = s.cacheInstance.Get(ctx, "pull_key")
	assert.ErrorIs(s.T(), err, ErrNotFound)
	assert.Nil(s.T(), value)
}

func (s *Suite) TestPullNotFound() {
	ctx := context.Background()

	value, err := s.cacheInstance.Pull(ctx, "missing_pull_key")
	assert.ErrorIs(s.T(), err, ErrNotFound)
	assert.Nil(s.T(), value)
}

func (s *Suite) TestPullIsSingleUseUnderConcurrency() {
	ctx := context.Background()

	require.NoError(s.T(), s.cacheInstance.Set(ctx, "one_shot", "the_value"))

	const workers = 25

	var (
		wg       sync.WaitGroup
		winners  atomic.Int64
		notFound atomic.Int64
	)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()

			value, err := s.cacheInstance.Pull(ctx, "one_shot")
			switch {
			case err == nil:
				winners.Add(1)
				assert.Equal(s.T(), "the_value", value)
			case errors.Is(err, ErrNotFound):
				notFound.Add(1)
			default:
				assert.NoError(s.T(), err)
			}
		}()
	}
	wg.Wait()

	assert.Equal(s.T(), int64(1), winners.Load(), "exactly one Pull must win the value")
	assert.Equal(s.T(), int64(workers-1), notFound.Load(), "every other Pull must get ErrNotFound")
}

func (s *Suite) TestClear() {
	ctx := context.Background()

	keys := []string{"clear_key1", "clear_key2", "clear_key3"}
	for _, key := range keys {
		err := s.cacheInstance.Set(ctx, key, "value_"+key)
		require.NoError(s.T(), err)
	}

	for _, key := range keys {
		value, err := s.cacheInstance.Get(ctx, key)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "value_"+key, value)
	}

	err := s.cacheInstance.Clear(ctx)
	require.NoError(s.T(), err)

	for _, key := range keys {
		value, err := s.cacheInstance.Get(ctx, key)
		assert.ErrorIs(s.T(), err, ErrNotFound)
		assert.Nil(s.T(), value)
	}
}

func (s *Suite) TestConcurrentAccess() {
	ctx := context.Background()
	done := make(chan bool, 3)

	go func() {
		for i := range 10 {
			key := "concurrent_key_1"
			err := s.cacheInstance.Set(ctx, key, i)
			assert.NoError(s.T(), err)
		}
		done <- true
	}()

	go func() {
		for i := range 10 {
			key := "concurrent_key_2"
			err := s.cacheInstance.Set(ctx, key, i*10)
			assert.NoError(s.T(), err)
		}
		done <- true
	}()

	go func() {
		for range 10 {
			_, _ = s.cacheInstance.Get(ctx, "concurrent_key_1")
			_, _ = s.cacheInstance.Get(ctx, "concurrent_key_2")
		}
		done <- true
	}()

	for range 3 {
		<-done
	}

	value1, _ := s.cacheInstance.Get(ctx, "concurrent_key_1")
	if value1 != nil {
		// Handle both int (in-memory) and float64 (JSON)
		if _, ok := value1.(float64); ok {
			assert.IsType(s.T(), float64(0), value1)
		} else {
			assert.IsType(s.T(), int(0), value1)
		}
	}

	value2, _ := s.cacheInstance.Get(ctx, "concurrent_key_2")
	if value2 != nil {
		// Handle both int (in-memory) and float64 (JSON)
		if _, ok := value2.(float64); ok {
			assert.IsType(s.T(), float64(0), value2)
		} else {
			assert.IsType(s.T(), int(0), value2)
		}
	}
}

func (s *Suite) TestSpecialCharacters() {
	ctx := context.Background()

	specialKeys := []string{
		"key with spaces",
		"key-with-dashes",
		"key_with_underscores",
		"key.with.dots",
		"key:with:colons",
		"key/with/slashes",
		"key\\with\\backslashes",
		"key'with'quotes",
		`key"with"double"quotes`,
	}

	for _, key := range specialKeys {
		s.T().Run(key, func(t *testing.T) {
			err := s.cacheInstance.Set(ctx, key, "special_value")
			require.NoError(t, err)

			value, err := s.cacheInstance.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, "special_value", value)

			err = s.cacheInstance.Delete(ctx, key)
			require.NoError(t, err)
		})
	}
}

func (s *Suite) TestLargeValue() {
	ctx := context.Background()

	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte('A' + (i % 26))
	}
	largeString := string(largeData)

	err := s.cacheInstance.Set(ctx, "large_key", largeString)
	require.NoError(s.T(), err)

	retrievedValue, err := s.cacheInstance.Get(ctx, "large_key")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), largeString, retrievedValue.(string))
}

func (s *Suite) TestEmptyValue() {
	ctx := context.Background()

	err := s.cacheInstance.Set(ctx, "empty_string", "")
	require.NoError(s.T(), err)

	value, err := s.cacheInstance.Get(ctx, "empty_string")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	err = s.cacheInstance.Set(ctx, "empty_map", map[string]any{})
	require.NoError(s.T(), err)

	value, err = s.cacheInstance.Get(ctx, "empty_map")
	require.NoError(s.T(), err)
	assert.IsType(s.T(), map[string]any{}, value)
	assert.Empty(s.T(), value)

	err = s.cacheInstance.Set(ctx, "empty_slice", []string{})
	require.NoError(s.T(), err)

	value, err = s.cacheInstance.Get(ctx, "empty_slice")
	require.NoError(s.T(), err)
	// Handle both []string (in-memory) and []any (JSON)
	if _, ok := value.([]any); ok {
		assert.IsType(s.T(), []any{}, value)
	} else {
		assert.IsType(s.T(), []string{}, value)
	}
	assert.Empty(s.T(), value)
}

func (s *Suite) TestNilValue() {
	ctx := context.Background()

	err := s.cacheInstance.Set(ctx, "nil_key", nil)
	require.NoError(s.T(), err)

	value, err := s.cacheInstance.Get(ctx, "nil_key")
	require.NoError(s.T(), err)
	assert.Nil(s.T(), value)
}
