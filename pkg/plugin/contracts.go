package plugin

import (
	"net/http"

	"github.com/gameap/gameap/pkg/plugin/proto"
)

// FileRefRequest is everything the host needs to stream the node file a
// plugin answered with (HTTPResponse.file) instead of a body.
type FileRefRequest struct {
	// PluginID is the database ID the plugin was loaded for; 0 (transient
	// load) is never granted anything.
	PluginID uint64
	// PluginName is the plugin's declared ID, for logs.
	PluginName string
	Ref        *proto.FileRef
	// Headers are the plugin-provided response headers; the panel applies
	// them before the ones it owns (Content-Length, Content-Disposition).
	Headers map[string]string
	// StatusCode is the plugin's status; 0 means 200.
	StatusCode int
}

// FileRefServer streams a node file referenced by a plugin HTTP response.
// It returns an error only while nothing has been written yet (the handler
// then answers with the error's HTTP status); a failure after the response
// started is logged and reported as nil.
type FileRefServer interface {
	ServeFileRef(w http.ResponseWriter, r *http.Request, req FileRefRequest) error
}
