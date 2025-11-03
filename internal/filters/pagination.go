package filters

const (
	DefaultLimit  = 20
	DefaultOffset = 0
)

var DefaultPagination = &Pagination{
	Limit:  DefaultLimit,
	Offset: DefaultOffset,
}

type Pagination struct {
	Limit  int
	Offset int
}

func NewPagination(limit, offset int) *Pagination {
	return &Pagination{
		Limit:  limit,
		Offset: offset,
	}
}
