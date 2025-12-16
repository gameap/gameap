package http //nolint:revive

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/http/http.proto
