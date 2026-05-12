package proto

//go:generate protoc -I../.. --go_out=../.. --go_opt=paths=source_relative --go-grpc_out=../.. --go-grpc_opt=paths=source_relative --go-vtproto_out=../.. --go-vtproto_opt=paths=source_relative,features=marshal+unmarshal+size pkg/proto/entity.proto pkg/proto/game.proto pkg/proto/gamemod.proto pkg/proto/daemontask.proto pkg/proto/node.proto pkg/proto/pat.proto pkg/proto/server.proto pkg/proto/serversetting.proto pkg/proto/servertask.proto pkg/proto/user.proto pkg/proto/gateway.proto pkg/proto/filetransfer.proto
