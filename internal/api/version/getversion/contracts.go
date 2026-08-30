package getversion

import (
	"context"

	"github.com/gameap/gameap/internal/services/releases"
)

// releasesService resolves the latest available releases of a component.
type releasesService interface {
	Latest(ctx context.Context, component releases.Component) (releases.Info, error)
	Enabled() bool
}
