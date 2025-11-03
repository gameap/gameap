package services

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

type transactionManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) (err error)
}

type globalAPIService interface {
	Games(ctx context.Context) ([]domain.GlobalAPIGame, error)
}

type GameUpgradeService struct {
	globalAPIService globalAPIService
	gameRepo         repositories.GameRepository
	gameModRepo      repositories.GameModRepository
	tm               transactionManager
}

func NewGameUpgradeService(
	globalAPIService globalAPIService,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	tm transactionManager,
) *GameUpgradeService {
	return &GameUpgradeService{
		globalAPIService: globalAPIService,
		gameRepo:         gameRepo,
		gameModRepo:      gameModRepo,
		tm:               tm,
	}
}

func (s *GameUpgradeService) UpgradeGames(ctx context.Context) error {
	apiGames, err := s.globalAPIService.Games(ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to fetch games from global api")
	}

	err = s.tm.Do(ctx, func(ctx context.Context) error {
		for _, apiGame := range apiGames {
			game := apiGame.ToDomainGame()

			err := s.gameRepo.Save(ctx, game)
			if err != nil {
				return errors.WithMessage(err, "failed to save game")
			}

			for _, apiMod := range apiGame.Mods {
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

					gameMod.Merge(apiMod.ToDomainGameMod())
				} else {
					gameMod = apiMod.ToDomainGameMod()
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
