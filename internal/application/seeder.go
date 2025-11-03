package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/samber/lo"
)

func seed(ctx context.Context, c *Container) error {
	err := seedClientCertificates(ctx, c)
	if err != nil {
		return errors.WithMessage(err, "failed to seed client certificates")
	}

	err = seedGamesAndMods(ctx, c)
	if err != nil {
		return errors.WithMessage(err, "failed to seed games")
	}

	err = seedRoles(ctx, c)
	if err != nil {
		return errors.WithMessage(err, "failed to seed permissions")
	}

	err = seedUsers(ctx, c)
	if err != nil {
		return errors.WithMessage(err, "failed to seed users")
	}

	return nil
}

func seedClientCertificates(ctx context.Context, c *Container) error {
	repo := c.ClientCertificateRepository()

	certs, err := repo.FindAll(ctx, nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to find certificates")
	}

	if len(certs) > 0 {
		// Certificates already exist, no need to seed.
		return nil
	}

	certService := c.CertificatesService()

	certName := xid.New().String()

	certPath := filepath.Join(certificates.ClientCertificatesPath, certName+".crt")
	keyPath := filepath.Join(certificates.ClientCertificatesPath, certName+".key")

	clientCert, _, err := certService.Generate(ctx, certPath, keyPath, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to generate client certificate")
	}

	fingerprint, err := certService.Fingerprint(clientCert)
	if err != nil {
		return errors.WithMessage(err, "failed to fingerprint client certificate")
	}

	clientCertificate := domain.ClientCertificate{
		Certificate: certPath,
		PrivateKey:  keyPath,
		Fingerprint: fingerprint,
		Expires:     time.Now().Add(certificates.CertYears * 365 * 24 * time.Hour),
	}

	if err := repo.Save(ctx, &clientCertificate); err != nil {
		return errors.WithMessage(err, "failed to save client certificate")
	}

	slog.InfoContext(ctx, "Client certificate seeded successfully", "fingerprint", clientCertificate.Fingerprint)

	return nil
}

func seedRoles(ctx context.Context, c *Container) error {
	return c.TransactionManager().Do(ctx, func(ctx context.Context) error {
		repo := c.RBACRepository()

		roles, err := repo.GetRoles(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to find roles")
		}

		if len(roles) > 0 {
			return nil
		}

		adminRole := domain.Role{Name: "admin", Title: lo.ToPtr("Administrator")}
		userRole := domain.Role{Name: "user", Title: lo.ToPtr("User")}

		if err := repo.SaveRole(ctx, &adminRole); err != nil {
			return errors.WithMessagef(err, "failed to save role %s", adminRole.Name)
		}

		if err := repo.SaveRole(ctx, &userRole); err != nil {
			return errors.WithMessagef(err, "failed to save role %s", userRole.Name)
		}

		err = repo.Allow(
			ctx, adminRole.ID, domain.EntityTypeRole, []domain.Ability{
				{
					Name:  domain.AbilityNameAdminRolesPermissions,
					Title: lo.ToPtr("Common Admininstator Permissions"),
				},
			},
		)
		if err != nil {
			return errors.WithMessage(err, "failed to assign abilities to admin role")
		}

		slog.InfoContext(ctx, "Roles seeded successfully")

		return nil
	})
}

func seedUsers(ctx context.Context, c *Container) error {
	return c.TransactionManager().Do(ctx, func(ctx context.Context) error {
		repo := c.UserRepository()

		users, err := repo.FindAll(ctx, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find users")
		}

		if len(users) > 0 {
			return nil
		}

		user := domain.User{
			Login: lo.CoalesceOrEmpty(os.Getenv("ADMIN_LOGIN"), "admin"),
			Email: lo.CoalesceOrEmpty(os.Getenv("ADMIN_EMAIL"), "admin@localhost"),
			Name:  lo.ToPtr("Admin"),
		}

		pw := os.Getenv("ADMIN_PASSWORD")

		if pw == "" {
			pw, err = strings.CryptoRandomString(12)
			if err != nil {
				return errors.WithMessage(err, "failed to generate random admin password")
			}

			slog.InfoContext(
				ctx,
				fmt.Sprintf(
					"ADMIN PASSWORD GENERATED\n\n"+
						"ADMIN PASSWORD: %s\n\n", pw,
				),
			)
		}

		user.Password, err = auth.HashPassword(pw)
		if err != nil {
			return errors.WithMessage(err, "failed to hash admin password")
		}

		if err := repo.Save(ctx, &user); err != nil {
			return errors.WithMessage(err, "failed to save user")
		}

		err = c.RBAC().SetRolesToUser(ctx, user.ID, []string{"admin"})
		if err != nil {
			return errors.WithMessage(err, "failed to assign admin role to user")
		}

		slog.InfoContext(ctx, "Admin user seeded successfully", "login", user.Login, "email", user.Email)

		return nil
	})
}

func seedGamesAndMods(ctx context.Context, c *Container) error {
	return c.TransactionManager().Do(ctx, func(ctx context.Context) error {
		games, err := c.GameRepository().FindAll(ctx, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find games")
		}

		if len(games) > 0 {
			return nil
		}

		err = c.GameUpgradeService().UpgradeGames(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to upgrade games")
		}

		slog.InfoContext(ctx, "Games and mods seeded successfully")

		return nil
	})
}
