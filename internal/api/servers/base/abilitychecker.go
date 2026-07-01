package base

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

// packageAuditLogger is the audit sink used by every AbilityChecker that was
// not given an explicit one. It mirrors the project's existing reliance on a
// process-wide logger (slog.Default) and lets the ~38 handler call sites that
// build a checker keep doing NewAbilityChecker(rbac) unchanged while still
// emitting access-denied events. Set once at startup via SetAuditLogger.
var packageAuditLogger audit.Logger = audit.NopLogger{}

// SetAuditLogger installs the process-wide audit logger for ability checks.
// Call once during application/router setup.
func SetAuditLogger(l audit.Logger) {
	if l != nil {
		packageAuditLogger = l
	}
}

// AbilityChecker is responsible for checking server abilities.
type AbilityChecker struct {
	rbac  base.RBAC
	audit audit.Logger
}

// AbilityCheckerOption customises an AbilityChecker at construction time.
type AbilityCheckerOption func(*AbilityChecker)

// WithAuditLogger overrides the audit logger for this checker (used in tests
// and where explicit injection is preferred over the package default).
func WithAuditLogger(l audit.Logger) AbilityCheckerOption {
	return func(c *AbilityChecker) {
		if l != nil {
			c.audit = l
		}
	}
}

func NewAbilityChecker(rbac base.RBAC, opts ...AbilityCheckerOption) *AbilityChecker {
	c := &AbilityChecker{
		rbac:  rbac,
		audit: packageAuditLogger,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Check checks if a user has all the specified abilities for a server.
// It returns true if the user is an admin or has all the specific abilities for the server.
func (c *AbilityChecker) Check(
	ctx context.Context,
	userID uint,
	serverID uint,
	abilities []domain.AbilityName,
) (bool, error) {
	isAdmin, err := c.rbac.Can(ctx, userID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return false, errors.WithMessage(err, "failed to check admin permissions")
	}

	if isAdmin {
		return true, nil
	}

	hasAbility, err := c.rbac.CanForEntity(
		ctx,
		userID,
		domain.EntityTypeServer,
		serverID,
		abilities,
	)
	if err != nil {
		return false, errors.WithMessage(err, "failed to check ability")
	}

	return hasAbility, nil
}

// CheckOrError checks if a user has all the specified abilities for a server.
// It returns an error if the user doesn't have all the abilities.
func (c *AbilityChecker) CheckOrError(
	ctx context.Context,
	userID uint,
	serverID uint,
	abilities []domain.AbilityName,
) error {
	hasAbility, err := c.Check(ctx, userID, serverID, abilities)
	if err != nil {
		return err
	}

	if !hasAbility {
		audit.AccessDenied(
			ctx,
			c.audit,
			"server",
			strconv.FormatUint(uint64(serverID), 10),
			"missing_ability",
			slog.String("required_abilities", joinAbilities(abilities)),
		)

		return api.WrapHTTPError(
			errors.Errorf("user does not have required permissions"),
			http.StatusForbidden,
		)
	}

	return nil
}

// CheckAny checks if a user has at least one of the specified abilities for a server.
// It returns true if the user is an admin or holds any of the abilities for the server.
func (c *AbilityChecker) CheckAny(
	ctx context.Context,
	userID uint,
	serverID uint,
	abilities []domain.AbilityName,
) (bool, error) {
	isAdmin, err := c.rbac.Can(ctx, userID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return false, errors.WithMessage(err, "failed to check admin permissions")
	}

	if isAdmin {
		return true, nil
	}

	for _, ability := range abilities {
		hasAbility, err := c.rbac.CanForEntity(
			ctx,
			userID,
			domain.EntityTypeServer,
			serverID,
			[]domain.AbilityName{ability},
		)
		if err != nil {
			return false, errors.WithMessage(err, "failed to check ability")
		}

		if hasAbility {
			return true, nil
		}
	}

	return false, nil
}

// CheckAnyOrError checks if a user has at least one of the specified abilities for a server.
// It returns an error if the user holds none of the abilities.
func (c *AbilityChecker) CheckAnyOrError(
	ctx context.Context,
	userID uint,
	serverID uint,
	abilities []domain.AbilityName,
) error {
	hasAbility, err := c.CheckAny(ctx, userID, serverID, abilities)
	if err != nil {
		return err
	}

	if !hasAbility {
		audit.AccessDenied(
			ctx,
			c.audit,
			"server",
			strconv.FormatUint(uint64(serverID), 10),
			"missing_ability",
			slog.String("required_abilities", joinAbilities(abilities)),
		)

		return api.WrapHTTPError(
			errors.Errorf("user does not have required permissions"),
			http.StatusForbidden,
		)
	}

	return nil
}

func joinAbilities(abilities []domain.AbilityName) string {
	parts := make([]string, len(abilities))
	for i, a := range abilities {
		parts[i] = string(a)
	}

	return strings.Join(parts, ",")
}
