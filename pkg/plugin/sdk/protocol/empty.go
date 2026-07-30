package protocol

import (
	"context"
	"errors"
)

// EmptyProtocolService provides no-op defaults for the ProtocolService
// interface. Plugins embed it and override only the methods they need — for
// example a declarative registration only needs GetRconProtocols and
// (optionally) ParsePlayers.
type EmptyProtocolService struct{}

func (EmptyProtocolService) GetRconProtocols(
	context.Context,
	*GetRconProtocolsRequest,
) (*GetRconProtocolsResponse, error) {
	return &GetRconProtocolsResponse{}, nil
}

func (EmptyProtocolService) GetQueryProtocols(
	context.Context,
	*GetQueryProtocolsRequest,
) (*GetQueryProtocolsResponse, error) {
	return &GetQueryProtocolsResponse{}, nil
}

func (EmptyProtocolService) RconOpen(context.Context, *RconOpenRequest) (*RconOpenResponse, error) {
	return nil, errors.New("not implemented") //nolint:err113
}

func (EmptyProtocolService) RconExecute(context.Context, *RconExecuteRequest) (*RconExecuteResponse, error) {
	return nil, errors.New("not implemented") //nolint:err113
}

func (EmptyProtocolService) RconClose(context.Context, *RconCloseRequest) (*RconCloseResponse, error) {
	return &RconCloseResponse{}, nil
}

func (EmptyProtocolService) QueryServer(context.Context, *QueryServerRequest) (*QueryServerResponse, error) {
	return nil, errors.New("not implemented") //nolint:err113
}

func (EmptyProtocolService) ParsePlayers(context.Context, *ParsePlayersRequest) (*ParsePlayersResponse, error) {
	return nil, errors.New("not implemented") //nolint:err113
}
