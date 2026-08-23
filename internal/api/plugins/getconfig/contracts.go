package getconfig

import (
	"github.com/gameap/gameap/internal/domain"
)

// DBIDResolver maps the ID a loaded plugin is registered under (its declared
// info ID) to its database ID. Optional: the loader satisfies it.
type DBIDResolver interface {
	GetDBPluginID(managerID string) (domain.Uint64ID, bool)
}
