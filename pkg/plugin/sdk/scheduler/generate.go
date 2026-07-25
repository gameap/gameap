package scheduler

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/scheduler/scheduler.proto
