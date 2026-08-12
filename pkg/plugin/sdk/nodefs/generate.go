package nodefs

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/nodefs/nodefs.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file nodefs_plugin.pb.go
