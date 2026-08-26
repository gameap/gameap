package secrets

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/secrets/secrets.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file secrets_plugin.pb.go
