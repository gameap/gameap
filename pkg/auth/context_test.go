package auth_test

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserFromContext(t *testing.T) {
	testUser := &domain.User{
		ID:    1,
		Login: "testuser",
		Email: "test@example.com",
	}

	// Test with user in context
	ctx := auth.ContextWithSession(context.Background(), &auth.Session{
		Login: testUser.Login,
		Email: testUser.Email,
	})
	session := auth.SessionFromContext(ctx)
	require.NotNil(t, session)
	assert.Equal(t, testUser.Login, session.Login)
	assert.Equal(t, testUser.Email, session.Email)

	// Test without user in context
	ctx = context.Background()
	session = auth.SessionFromContext(ctx)
	assert.Nil(t, session)
}
