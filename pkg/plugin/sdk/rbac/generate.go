package rbac

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/rbac/rbac.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file rbac_plugin.pb.go
