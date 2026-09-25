package idempotency

import (
	"bytes"
	"net/http"
	"slices"
)

// replayedHeaders are the response headers stored with an outcome. The rest
// (CORS, X-Request-Id, cookies) belongs to the request that gets the replay.
//
//nolint:gochecknoglobals
var replayedHeaders = []string{"Content-Type", "Location", "Etag", "Last-Modified", "Cache-Control"}

// recorder passes the response through to the client while keeping a copy of
// the status, the replayed headers and the body (up to limit bytes).
type recorder struct {
	http.ResponseWriter

	limit int

	wroteHeader bool
	status      int
	header      http.Header
	body        bytes.Buffer
	overflow    bool
}

func newRecorder(w http.ResponseWriter, limit int) *recorder {
	return &recorder{
		ResponseWriter: w,
		limit:          limit,
	}
}

func (r *recorder) WriteHeader(code int) {
	// Informational responses precede the final one and are not stored.
	if !r.wroteHeader && code >= http.StatusContinue && code < http.StatusOK {
		r.ResponseWriter.WriteHeader(code)

		return
	}

	if !r.wroteHeader {
		r.capture(code)
	}

	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.capture(http.StatusOK)
	}

	if !r.overflow {
		if r.body.Len()+len(p) > r.limit {
			r.overflow = true
			r.body = bytes.Buffer{}
		} else {
			r.body.Write(p)
		}
	}

	return r.ResponseWriter.Write(p)
}

func (r *recorder) Flush() {
	if !r.wroteHeader {
		r.capture(http.StatusOK)
	}

	_ = http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// finish records what net/http sends for a handler that wrote nothing: 200
// with the headers set by then.
func (r *recorder) finish() {
	if !r.wroteHeader {
		r.capture(http.StatusOK)
	}
}

// capture snapshots the headers when net/http commits them: a header set after
// WriteHeader never reaches the client, so it must not be replayed either.
func (r *recorder) capture(status int) {
	r.wroteHeader = true
	r.status = status
	r.header = make(http.Header, len(replayedHeaders))

	for _, name := range replayedHeaders {
		if values := r.ResponseWriter.Header().Values(name); len(values) > 0 {
			r.header[name] = slices.Clone(values)
		}
	}
}
