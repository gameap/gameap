package getservers

import (
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/filters"
)

func parseFilters(r *http.Request) *filters.FindServer {
	query := r.URL.Query()

	filter := &filters.FindServer{}

	if idFilter := query.Get("filter[id]"); idFilter != "" {
		if id, err := strconv.ParseUint(idFilter, 10, 64); err == nil {
			filter.IDs = []uint{uint(id)}
		}
	}

	return filter
}
