package base

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/domain"
)

//go:generate mockgen -source=./contracts.go -destination=./mocks/contracts_mock.go -package=mocks

type Responder interface {
	WriteError(ctx context.Context, rw http.ResponseWriter, err error)
	Write(ctx context.Context, rw http.ResponseWriter, result any)
}

type RBAC interface {
	Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
	CanOneOf(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
	CanForEntity(
		ctx context.Context,
		userID uint,
		entityType domain.EntityType,
		entityID uint,
		abilities []domain.AbilityName,
	) (bool, error)

	GetRoles(ctx context.Context, userID uint) ([]string, error)

	SetRolesToUser(ctx context.Context, userID uint, roleNames []string) error

	AllowUserAbilitiesForEntity(
		ctx context.Context,
		userID uint,
		entityID uint,
		entityType domain.EntityType,
		abilityNames []domain.AbilityName,
	) error

	RevokeOrForbidUserAbilitiesForEntity(
		ctx context.Context,
		userID uint,
		entityID uint,
		entityType domain.EntityType,
		abilityNames []domain.AbilityName,
	) error
}

type TransactionManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) (err error)
}
