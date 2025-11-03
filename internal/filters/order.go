package filters

import "fmt"

type SortDirection int

const (
	SortDirectionAsc SortDirection = iota
	SortDirectionDesc
)

func (sd SortDirection) String() string {
	switch sd {
	case SortDirectionAsc:
		return "asc"
	case SortDirectionDesc:
		return "desc"
	default:
		return ""
	}
}

type Sorting struct {
	Field     string
	Direction SortDirection
}

func NewSorting(field string, direction SortDirection) *Sorting {
	return &Sorting{
		Field:     field,
		Direction: direction,
	}
}

func (p *Sorting) String() string {
	return fmt.Sprintf("%s %s", p.Field, p.Direction.String())
}
