package filters

import "github.com/gameap/gameap/internal/domain"

type FindNode struct {
	IDs             []uint
	Enabled         *bool
	OS              *domain.NodeOS
	GDaemonAPIKey   *string
	GDaemonAPIToken *string
	WithDeleted     bool
}

func FindNodeByIDs(ids ...uint) *FindNode {
	return &FindNode{
		IDs: ids,
	}
}

func FindNodeByGDaemonAPIKey(key string) *FindNode {
	return &FindNode{
		GDaemonAPIKey: &key,
	}
}
