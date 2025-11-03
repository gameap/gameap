package filters

type FindGameMod struct {
	IDs       []uint
	Names     []string
	GameCodes []string
}

// FilterCount returns the number of different filter types being used (not the total number of values).
// Useful for determining if the filter is specific enough for caching (e.g., FilterCount() == 1 means
// only one filter type is used, making it suitable for single-key caching).
// Example: filter with IDs=[1,2,3] and Names=["mod1"] returns 2, not 4.
func (f *FindGameMod) FilterCount() int {
	count := 0
	if len(f.IDs) > 0 {
		count++
	}

	if len(f.Names) > 0 {
		count++
	}

	if len(f.GameCodes) > 0 {
		count++
	}

	return count
}

func FindGameModByGameCodes(codes ...string) *FindGameMod {
	return &FindGameMod{
		GameCodes: codes,
	}
}
