package storage

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/storage/storage.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file storage_plugin.pb.go
