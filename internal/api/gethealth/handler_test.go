package gethealth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type mockResponder struct {
	writeCalled      bool
	writeErrorCalled bool
	lastResult       any
	lastError        error
}

func (m *mockResponder) WriteError(_ context.Context, _ http.ResponseWriter, err error) {
	m.writeErrorCalled = true
	m.lastError = err
}

func (m *mockResponder) Write(_ context.Context, _ http.ResponseWriter, result any) {
	m.writeCalled = true
	m.lastResult = result
}

func TestNewGetHealthHandler(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	responder := &mockResponder{}

	handler := NewGetHealthHandler(db, responder)

	require.NotNil(t, handler)
	assert.Equal(t, db, handler.db)
	assert.Equal(t, responder, handler.responder)
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		setupDB          func() *sql.DB
		expectWriteError bool
		expectWrite      bool
	}{
		{
			name: "successful_health_check",
			setupDB: func() *sql.DB {
				db, _ := sql.Open("sqlite", "file::memory:?cache=shared")

				return db
			},
			expectWriteError: false,
			expectWrite:      true,
		},
		{
			name: "database_ping_error",
			setupDB: func() *sql.DB {
				db, _ := sql.Open("sqlite", "file::memory:?cache=shared")
				db.Close()

				return db
			},
			expectWriteError: true,
			expectWrite:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := tt.setupDB()

			responder := &mockResponder{}
			handler := NewGetHealthHandler(db, responder)

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectWriteError, responder.writeErrorCalled)
			assert.Equal(t, tt.expectWrite, responder.writeCalled)

			if !tt.expectWriteError {
				db.Close()
			}
		})
	}
}

func TestHandler_ServeHTTP_with_context(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	require.NoError(t, err)
	defer db.Close()

	responder := &mockResponder{}
	handler := NewGetHealthHandler(db, responder)

	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.False(t, responder.writeErrorCalled)
	assert.True(t, responder.writeCalled)
}
