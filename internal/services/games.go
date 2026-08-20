package services

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

type transactionManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) (err error)
}

type gamesProvider interface {
	Games(ctx context.Context) ([]domain.GlobalAPIGame, error)
}

type GameUpgradeService struct {
	gamesProvider gamesProvider
	gameRepo      repositories.GameRepository
	gameModRepo   repositories.GameModRepository
	tm            transactionManager
}

func NewGameUpgradeService(
	gamesProvider gamesProvider,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	tm transactionManager,
) *GameUpgradeService {
	return &GameUpgradeService{
		gamesProvider: gamesProvider,
		gameRepo:      gameRepo,
		gameModRepo:   gameModRepo,
		tm:            tm,
	}
}

func (s *GameUpgradeService) UpgradeGames(ctx context.Context) error {
	apiGames, err := s.gamesProvider.Games(ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to fetch games")
	}

	err = s.tm.Do(ctx, func(ctx context.Context) error {
		for _, apiGame := range apiGames {
			game := apiGame.ToDomainGame()

			err := s.gameRepo.Save(ctx, game)
			if err != nil {
				return errors.WithMessage(err, "failed to save game")
			}

			for _, apiMod := range apiGame.Mods {
				incoming := apiMod.ToDomainGameMod()
				if !sanitizeCatalogMod(ctx, incoming) {
					continue
				}

				gameMods, err := s.gameModRepo.Find(ctx, &filters.FindGameMod{
					Names:     []string{apiMod.Name},
					GameCodes: []string{apiMod.GameCode},
				}, nil, nil)
				if err != nil {
					return errors.WithMessage(err, "failed to find game mod")
				}

				if len(gameMods) > 1 {
					continue
				}

				var gameMod *domain.GameMod

				if len(gameMods) == 1 {
					gameMod = &gameMods[0]

					gameMod.Merge(incoming)
				} else {
					gameMod = incoming
				}

				err = s.gameModRepo.Save(ctx, gameMod)
				if err != nil {
					return errors.WithMessage(err, "failed to save game mod")
				}
			}
		}

		return nil
	})

	return err
}

// sanitizeCatalogMod normalizes the definitions that came from the catalog and
// reports whether the mod is safe to store. A single malformed entry is skipped
// with a warning rather than aborting the whole upgrade.
func sanitizeCatalogMod(ctx context.Context, gameMod *domain.GameMod) bool {
	for i := range gameMod.Vars {
		gameMod.Vars[i].Normalize()
	}

	for i := range gameMod.FastRcon {
		gameMod.FastRcon[i].Normalize()
	}

	if err := gameMod.Vars.Validate(); err != nil {
		slog.WarnContext(ctx, "skipping game mod with invalid variables",
			slog.String("game_code", gameMod.GameCode),
			slog.String("mod", gameMod.Name),
			slog.String("error", err.Error()),
		)

		return false
	}

	if err := gameMod.FastRcon.Validate(); err != nil {
		slog.WarnContext(ctx, "skipping game mod with invalid fast rcon commands",
			slog.String("game_code", gameMod.GameCode),
			slog.String("mod", gameMod.Name),
			slog.String("error", err.Error()),
		)

		return false
	}

	return true
}
