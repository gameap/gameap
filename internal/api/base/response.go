package base

var Success = struct {
	Status string `json:"status"`
}{
	Status: "ok",
}

type PaginatedResponse[T any] struct {
	CurrentPage int `json:"current_page"`
	Data        []T `json:"data"`
	From        int `json:"from"`
	LastPage    int `json:"last_page"`
	PerPage     int `json:"per_page"`
	Total       int `json:"total"`
}

func NewPaginatedResponse[T any](data []T, currentPage, perPage, total int) *PaginatedResponse[T] {
	var from int
	if len(data) > 0 {
		from = (currentPage-1)*perPage + 1
	} else {
		from = 0
	}

	lastPage := 1
	if total > 0 {
		lastPage = (total + perPage - 1) / perPage
	}

	return &PaginatedResponse[T]{
		CurrentPage: currentPage,
		Data:        data,
		From:        from,
		LastPage:    lastPage,
		PerPage:     perPage,
		Total:       total,
	}
}
