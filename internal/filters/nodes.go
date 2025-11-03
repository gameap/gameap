package filters

type FindNode struct {
	IDs             []uint
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
