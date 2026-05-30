// OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
//
// Pins that the MFA-enrollment token carries the restricting "scope" claim
// while an ordinary session does not, so the enrollment scope guard can tell
// a forced-enrollment session from a full one.
package auth

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPASETOKey = "0123456789abcdef0123456789abcdef" // exactly 32 bytes

func TestPASETOService_GenerateMFAEnrollmentToken_CarriesScope(t *testing.T) {
	svc, err := NewPASETOService([]byte(testPASETOKey))
	require.NoError(t, err)

	token, err := svc.GenerateMFAEnrollmentToken(&domain.User{Login: "admin"}, time.Hour)
	require.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	require.NoError(t, err)

	scope, err := claims.GetScope()
	require.NoError(t, err)
	assert.Equal(t, ScopeMFAEnrollment, scope)
}

func TestPASETOService_GenerateTokenForUser_HasNoScope(t *testing.T) {
	svc, err := NewPASETOService([]byte(testPASETOKey))
	require.NoError(t, err)

	token, err := svc.GenerateTokenForUser(&domain.User{Login: "admin"}, time.Hour)
	require.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	require.NoError(t, err)

	scope, err := claims.GetScope()
	require.NoError(t, err)
	assert.Empty(t, scope, "an ordinary session token must not carry a scope claim")
}

func TestJWTService_GenerateMFAEnrollmentToken_CarriesScope(t *testing.T) {
	svc := NewJWTService([]byte("test-secret-key"))

	token, err := svc.GenerateMFAEnrollmentToken(&domain.User{Login: "admin"}, time.Hour)
	require.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	require.NoError(t, err)

	scope, err := claims.GetScope()
	require.NoError(t, err)
	assert.Equal(t, ScopeMFAEnrollment, scope)
}

func TestJWTService_GenerateTokenForUser_HasNoScope(t *testing.T) {
	svc := NewJWTService([]byte("test-secret-key"))

	token, err := svc.GenerateTokenForUser(&domain.User{Login: "admin"}, time.Hour)
	require.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	require.NoError(t, err)

	scope, err := claims.GetScope()
	require.NoError(t, err)
	assert.Empty(t, scope)
}
