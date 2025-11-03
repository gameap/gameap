package inmemory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
)

type GameRepository struct {
	mu    sync.RWMutex
	games map[string]*domain.Game
}

func NewGameRepository() *GameRepository {
	return &GameRepository{
		games: make(map[string]*domain.Game),
	}
}

func (r *GameRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	games := make([]domain.Game, 0, len(r.games))
	for _, game := range r.games {
		games = append(games, *game)
	}

	r.sortGames(games, order)

	return r.applyPagination(games, pagination), nil
}

func (r *GameRepository) Find(
	_ context.Context,
	filter *filters.FindGame,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Game, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var games []domain.Game

	if filter == nil || len(filter.Codes) == 0 {
		for _, game := range r.games {
			games = append(games, *game)
		}
	} else {
		codeSet := make(map[string]bool)
		for _, code := range filter.Codes {
			codeSet[code] = true
		}

		for _, game := range r.games {
			if codeSet[game.Code] {
				games = append(games, *game)
			}
		}
	}

	r.sortGames(games, order)

	return r.applyPagination(games, pagination), nil
}

func (r *GameRepository) Save(_ context.Context, game *domain.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.games[game.Code] = &domain.Game{
		Code:                    game.Code,
		Name:                    game.Name,
		Engine:                  game.Engine,
		EngineVersion:           game.EngineVersion,
		SteamAppIDLinux:         game.SteamAppIDLinux,
		SteamAppIDWindows:       game.SteamAppIDWindows,
		SteamAppSetConfig:       game.SteamAppSetConfig,
		RemoteRepositoryLinux:   game.RemoteRepositoryLinux,
		RemoteRepositoryWindows: game.RemoteRepositoryWindows,
		LocalRepositoryLinux:    game.LocalRepositoryLinux,
		LocalRepositoryWindows:  game.LocalRepositoryWindows,
		Enabled:                 game.Enabled,
	}

	return nil
}

func (r *GameRepository) Delete(_ context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.games, code)

	return nil
}

func (r *GameRepository) sortGames(games []domain.Game, order []filters.Sorting) {
	if len(order) == 0 {
		return
	}

	sort.Slice(games, func(i, j int) bool {
		for _, sorting := range order {
			var result int
			switch sorting.Field {
			case "code":
				result = strings.Compare(games[i].Code, games[j].Code)
			case "name":
				result = strings.Compare(games[i].Name, games[j].Name)
			default:
				continue
			}

			if result != 0 {
				if sorting.Direction == filters.SortDirectionDesc {
					return result > 0
				}

				return result < 0
			}
		}

		return false
	})
}

func (r *GameRepository) applyPagination(games []domain.Game, pagination *filters.Pagination) []domain.Game {
	if pagination == nil {
		return games
	}

	if pagination.Offset >= len(games) {
		return []domain.Game{}
	}

	start := pagination.Offset
	end := min(start+pagination.Limit, len(games))

	return games[start:end]
}
