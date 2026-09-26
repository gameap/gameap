package domain

import (
	"cmp"
	"iter"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	minPort = 1
	maxPort = 65535
)

var ErrPortRangeInvalid = errors.New(
	"port range must list ports and ranges within 1-65535, e.g. 27015-27100, 28000",
)

// PortSpan is an inclusive run of ports.
type PortSpan struct {
	From int
	To   int
}

// PortRange is a pool of ports written as comma-separated ports and inclusive
// ranges: "27015-27100, 28000". Spans are kept sorted and merged, so walking
// the pool visits every port once however the ranges overlap.
type PortRange []PortSpan

// ParsePortRange reads a pool; a blank string is an empty pool.
func ParsePortRange(s string) (PortRange, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	spans := make(PortRange, 0, len(parts))

	for _, part := range parts {
		span, err := parsePortSpan(part)
		if err != nil {
			return nil, errors.WithMessagef(err, "invalid entry %q", strings.TrimSpace(part))
		}

		spans = append(spans, span)
	}

	return mergePortSpans(spans), nil
}

// All yields the ports of the pool in ascending order.
func (r PortRange) All() iter.Seq[int] {
	return func(yield func(int) bool) {
		for _, span := range r {
			for port := span.From; port <= span.To; port++ {
				if !yield(port) {
					return
				}
			}
		}
	}
}

func parsePortSpan(s string) (PortSpan, error) {
	fromText, toText, isRange := strings.Cut(s, "-")

	from, err := parsePort(fromText)
	if err != nil {
		return PortSpan{}, err
	}

	to := from
	if isRange {
		to, err = parsePort(toText)
		if err != nil {
			return PortSpan{}, err
		}
	}

	if from > to {
		return PortSpan{}, ErrPortRangeInvalid
	}

	return PortSpan{From: from, To: to}, nil
}

func parsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, ErrPortRangeInvalid
	}

	port, err := strconv.Atoi(s)
	if err != nil || port < minPort || port > maxPort {
		return 0, ErrPortRangeInvalid
	}

	return port, nil
}

func mergePortSpans(spans PortRange) PortRange {
	slices.SortFunc(spans, func(a, b PortSpan) int {
		return cmp.Compare(a.From, b.From)
	})

	merged := make(PortRange, 0, len(spans))

	for _, span := range spans {
		if last := len(merged) - 1; last >= 0 && span.From <= merged[last].To+1 {
			merged[last].To = max(merged[last].To, span.To)

			continue
		}

		merged = append(merged, span)
	}

	return merged
}
