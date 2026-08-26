package host

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/host/host.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file host_plugin.pb.go
