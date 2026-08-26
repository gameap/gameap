package ssh

//go:generate protoc -I../../../.. --go-plugin_out=../../../.. --go-plugin_opt=paths=source_relative pkg/plugin/sdk/ssh/ssh.proto
//go:generate go run ../../../../scripts/patch_plugin_guest.go -file ssh_plugin.pb.go
