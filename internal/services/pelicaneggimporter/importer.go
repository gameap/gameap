package pelicaneggimporter

import (
	"context"
	"maps"
	"regexp"
	"strings"
	"unicode"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/domain/gamesimport"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type transactionManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) (err error)
}

type Importer struct {
	gameRepo    repositories.GameRepository
	gameModRepo repositories.GameModRepository
	tm          transactionManager
}

func NewImporter(
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	tm transactionManager,
) *Importer {
	return &Importer{
		gameRepo:    gameRepo,
		gameModRepo: gameModRepo,
		tm:          tm,
	}
}

// ImportResult contains the result of importing a Pelican egg.
type ImportResult struct {
	Game    *domain.Game
	GameMod *domain.GameMod
}

// Import imports a Pelican egg and creates/updates Game and GameMod entities.
func (i *Importer) Import(
	ctx context.Context,
	egg *gamesimport.PelicanEgg,
	opts *gamesimport.Options,
) (*ImportResult, error) {
	if egg == nil {
		return nil, errors.New("egg cannot be nil")
	}

	if egg.Name == "" {
		return nil, errors.New("egg name is required")
	}

	if err := opts.Validate(); err != nil {
		return nil, errors.WithMessage(err, "options validation failed")
	}

	game, gameMod := i.buildEntities(egg, opts)

	return i.saveEntities(ctx, game, gameMod)
}

func (i *Importer) buildEntities(
	egg *gamesimport.PelicanEgg,
	opts *gamesimport.Options,
) (*domain.Game, *domain.GameMod) {
	gameCode := generateGameCode(egg.Name)
	gameName := egg.Name

	if opts != nil {
		if opts.Code != nil {
			gameCode = *opts.Code
		}
		if opts.Name != nil {
			gameName = *opts.Name
		}
	}

	startCmd := transformStartupCommand(egg.GetStartupCommand())
	vars := transformVariables(egg.Variables)

	game := &domain.Game{
		Code:    gameCode,
		Name:    gameName,
		Engine:  "pelican",
		Enabled: 1,
		Metadata: domain.Metadata{
			"pelican_egg": egg.Raw,
		},
	}

	gameMod := &domain.GameMod{
		GameCode:      gameCode,
		Name:          "Default",
		StartCmdLinux: new(startCmd),
		Vars:          vars,
		Metadata:      buildGameModMetadata(egg),
	}

	return game, gameMod
}

func (i *Importer) saveEntities(
	ctx context.Context,
	game *domain.Game,
	gameMod *domain.GameMod,
) (*ImportResult, error) {
	var result ImportResult

	err := i.tm.Do(ctx, func(ctx context.Context) error {
		existingGames, err := i.gameRepo.Find(ctx, filters.FindGameByCodes(game.Code), nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find existing game")
		}

		if len(existingGames) > 0 {
			game.Metadata = mergeMetadata(existingGames[0].Metadata, game.Metadata)
		}

		if err := i.gameRepo.Save(ctx, game); err != nil {
			return errors.WithMessage(err, "failed to save game")
		}

		result.Game = game

		existingMods, err := i.gameModRepo.Find(ctx, &filters.FindGameMod{
			Names:     []string{"Default"},
			GameCodes: []string{game.Code},
		}, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find existing game mod")
		}

		if len(existingMods) > 0 {
			gameMod.ID = existingMods[0].ID
			gameMod.Metadata = mergeMetadata(existingMods[0].Metadata, gameMod.Metadata)
		}

		if err := i.gameModRepo.Save(ctx, gameMod); err != nil {
			return errors.WithMessage(err, "failed to save game mod")
		}

		result.GameMod = gameMod

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func generateGameCode(name string) string {
	code := slugify(name)

	code = strings.ReplaceAll(code, "-", "_")

	if len(code) > 16 {
		code = code[:16]
	}

	code = strings.TrimRight(code, "_")

	if len(code) < 2 {
		code = "gm"
	}

	return code
}

func slugify(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, s)

	normalized = strings.ToLower(normalized)

	var result strings.Builder
	prevDash := false

	for _, c := range normalized {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result.WriteRune(c)
			prevDash = false
		} else if !prevDash {
			result.WriteRune('-')
			prevDash = true
		}
	}

	return strings.Trim(result.String(), "-")
}

// transformStartupCommand transforms Pelican startup command format to GameAP format.
// Pelican format: ./server -port {{server.build.default.port}} -name {{SERVER_NAME}}.
// GameAP format: ./server -port {port} -name {SERVER_NAME}.
// If command contains shell operators (&&, ;, |, etc.), it wraps command in /bin/sh -c "...".
func transformStartupCommand(startup string) string {
	result := startup

	result = regexp.MustCompile(`\{\{server\.build\.default\.port\}\}`).ReplaceAllString(result, "{port}")
	result = regexp.MustCompile(`\{\{server\.build\.env\.PORT\}\}`).ReplaceAllString(result, "{port}")
	result = regexp.MustCompile(`\{\{server\.build\.default\.ip\}\}`).ReplaceAllString(result, "{ip}")
	result = regexp.MustCompile(`\{\{server\.build\.env\.SERVER_IP\}\}`).ReplaceAllString(result, "{ip}")

	envVarPattern := regexp.MustCompile(`\{\{([A-Z_][A-Z0-9_]*)\}\}`)
	result = envVarPattern.ReplaceAllString(result, "{$1}")

	serverEnvPattern := regexp.MustCompile(`\{\{server\.build\.env\.([A-Z_][A-Z0-9_]*)\}\}`)
	result = serverEnvPattern.ReplaceAllString(result, "{$1}")

	if needsShellWrapper(result) {
		result = wrapInShell(result)
	}

	return result
}

func needsShellWrapper(cmd string) bool {
	shellOperators := []string{"&&", "||", ";", "|", ">", "<", ">>", "<<", "&", "`", "$("}
	for _, op := range shellOperators {
		if strings.Contains(cmd, op) {
			return true
		}
	}

	return false
}

func wrapInShell(cmd string) string {
	escaped := strings.ReplaceAll(cmd, `"`, `\"`)

	return `/bin/sh -c "` + escaped + `"`
}

func buildGameModMetadata(egg *gamesimport.PelicanEgg) domain.Metadata {
	return domain.Metadata{
		"docker_image":                   egg.FirstDockerImage(),
		"docker_installation_script":     egg.Scripts.Installation.Script,
		"docker_installation_image":      egg.Scripts.Installation.Container,
		"docker_installation_entrypoint": egg.Scripts.Installation.Entrypoint,
		"docker_installation_user":       "root",
		"docker_startup_done":            egg.Config.Startup.Done,
		"docker_workdir":                 "/home/container",
		"pelican_egg":                    egg.Raw,
	}
}

func mergeMetadata(existing, updated domain.Metadata) domain.Metadata {
	if existing == nil {
		return updated
	}

	if updated == nil {
		return existing
	}

	result := make(domain.Metadata)
	maps.Copy(result, existing)
	maps.Copy(result, updated)

	return result
}
