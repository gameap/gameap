package authz

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/authz/authz.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file authz_plugin.pb.go
