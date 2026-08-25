package nodes

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/nodes/nodes.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file nodes_plugin.pb.go
