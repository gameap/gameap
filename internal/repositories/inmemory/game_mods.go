package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
)

type GameModRepository struct {
	mu       sync.RWMutex
	gameMods map[uint]*domain.GameMod
	nextID   uint
}

func NewGameModRepository() *GameModRepository {
	return &GameModRepository{
		gameMods: make(map[uint]*domain.GameMod),
		nextID:   1,
	}
}

func (r *GameModRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	gameMods := make([]domain.GameMod, 0, len(r.gameMods))
	for _, gameMod := range r.gameMods {
		gameMods = append(gameMods, *gameMod)
	}

	r.sortGameMods(gameMods, order)

	return r.applyPagination(gameMods, pagination), nil
}

func (r *GameModRepository) Find(
	_ context.Context,
	filter *filters.FindGameMod,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.GameMod, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	gameMods := make([]domain.GameMod, 0, len(r.gameMods))

	if filter == nil {
		for _, gameMod := range r.gameMods {
			gameMods = append(gameMods, *gameMod)
		}

		r.sortGameMods(gameMods, order)

		return r.applyPagination(gameMods, pagination), nil
	}

	idSet := make(map[uint]bool)
	for _, id := range filter.IDs {
		idSet[id] = true
	}

	gameCodeSet := make(map[string]bool)
	for _, gameCode := range filter.GameCodes {
		gameCodeSet[gameCode] = true
	}

	nameSet := make(map[string]bool)
	for _, name := range filter.Names {
		nameSet[name] = true
	}

	for _, gameMod := range r.gameMods {
		if len(filter.IDs) > 0 && !idSet[gameMod.ID] {
			continue
		}

		if len(filter.GameCodes) > 0 && !gameCodeSet[gameMod.GameCode] {
			continue
		}

		if len(filter.Names) > 0 && !nameSet[gameMod.Name] {
			continue
		}

		gameMods = append(gameMods, *gameMod)
	}

	r.sortGameMods(gameMods, order)

	return r.applyPagination(gameMods, pagination), nil
}

func (r *GameModRepository) Save(_ context.Context, gameMod *domain.GameMod) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if gameMod.ID == 0 {
		gameMod.ID = r.nextID
		r.nextID++
	}

	r.gameMods[gameMod.ID] = &domain.GameMod{
		ID:                      gameMod.ID,
		GameCode:                gameMod.GameCode,
		Name:                    gameMod.Name,
		FastRcon:                gameMod.FastRcon,
		Vars:                    gameMod.Vars,
		RemoteRepositoryLinux:   gameMod.RemoteRepositoryLinux,
		RemoteRepositoryWindows: gameMod.RemoteRepositoryWindows,
		LocalRepositoryLinux:    gameMod.LocalRepositoryLinux,
		LocalRepositoryWindows:  gameMod.LocalRepositoryWindows,
		StartCmdLinux:           gameMod.StartCmdLinux,
		StartCmdWindows:         gameMod.StartCmdWindows,
		KickCmd:                 gameMod.KickCmd,
		BanCmd:                  gameMod.BanCmd,
		ChnameCmd:               gameMod.ChnameCmd,
		SrestartCmd:             gameMod.SrestartCmd,
		ChmapCmd:                gameMod.ChmapCmd,
		SendmsgCmd:              gameMod.SendmsgCmd,
		PasswdCmd:               gameMod.PasswdCmd,
	}

	return nil
}

func (r *GameModRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.gameMods, id)

	return nil
}

func (r *GameModRepository) sortGameMods(gameMods []domain.GameMod, order []filters.Sorting) {
	if len(order) == 0 {
		return
	}

	sort.Slice(gameMods, func(i, j int) bool {
		for _, sorting := range order {
			var result int
			switch sorting.Field {
			case "id":
				result = cmp.Compare(gameMods[i].ID, gameMods[j].ID)
			case "game_code":
				result = strings.Compare(gameMods[i].GameCode, gameMods[j].GameCode)
			case "name":
				result = strings.Compare(gameMods[i].Name, gameMods[j].Name)
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

func (r *GameModRepository) applyPagination(
	gameMods []domain.GameMod,
	pagination *filters.Pagination,
) []domain.GameMod {
	if pagination == nil {
		return gameMods
	}

	if pagination.Offset >= len(gameMods) {
		return []domain.GameMod{}
	}

	start := pagination.Offset
	end := min(start+pagination.Limit, len(gameMods))

	return gameMods[start:end]
}
