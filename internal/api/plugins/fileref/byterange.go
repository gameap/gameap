package fileref

import (
	"strconv"
	"strings"
)

// byteRange is one resolved half-open-free range: both ends inclusive, both
// inside the file.
type byteRange struct {
	start uint64
	// length is always at least 1 — an empty range is not representable and
	// never produced.
	length uint64
}

// parseByteRange resolves a Range header against a known file size.
//
// Only a single range is honoured. A multi-range request would have to be
// answered as multipart/byteranges, and the one thing ranges are for here —
// resuming an interrupted download — never asks for more than one. An
// unsupported or malformed header answers ok = false, which the caller turns
// into an ordinary whole-file response, exactly as RFC 9110 allows.
//
// satisfiable = false marks a syntactically valid range that lies past the end
// of the file; that one owes the client a 416 rather than the whole file.
func parseByteRange(header string, size uint64) (rng byteRange, ok bool, satisfiable bool) {
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !found {
		return byteRange{}, false, false
	}

	spec = strings.TrimSpace(spec)
	if spec == "" || strings.Contains(spec, ",") {
		return byteRange{}, false, false
	}

	first, last, found := strings.Cut(spec, "-")
	if !found {
		return byteRange{}, false, false
	}

	first, last = strings.TrimSpace(first), strings.TrimSpace(last)

	// "bytes=-N": the final N bytes.
	if first == "" {
		suffix, err := strconv.ParseUint(last, 10, 64)
		if err != nil || suffix == 0 {
			return byteRange{}, false, false
		}

		if size == 0 {
			return byteRange{}, true, false
		}

		if suffix > size {
			suffix = size
		}

		return byteRange{start: size - suffix, length: suffix}, true, true
	}

	start, err := strconv.ParseUint(first, 10, 64)
	if err != nil {
		return byteRange{}, false, false
	}

	if start >= size {
		return byteRange{}, true, false
	}

	// "bytes=N-": from N to the end.
	if last == "" {
		return byteRange{start: start, length: size - start}, true, true
	}

	end, err := strconv.ParseUint(last, 10, 64)
	if err != nil || end < start {
		return byteRange{}, false, false
	}

	if end >= size {
		end = size - 1
	}

	return byteRange{start: start, length: end - start + 1}, true, true
}

// contentRange is the Content-Range value for a served range.
func (r byteRange) contentRange(size uint64) string {
	return "bytes " + strconv.FormatUint(r.start, 10) + "-" +
		strconv.FormatUint(r.start+r.length-1, 10) + "/" + strconv.FormatUint(size, 10)
}
