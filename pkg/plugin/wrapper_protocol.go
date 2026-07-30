package plugin

import (
	"context"

	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/pkg/errors"
)

// The module wrapper also implements the optional ProtocolService (RCON/Query
// protocol extension). The methods share the same guest-call gate as the core
// PluginService, so calls into a plugin stay serialized across both services.
var _ protocol.ProtocolService = (*pluginServiceWrapper)(nil)

func (p *pluginServiceWrapper) GetRconProtocols(
	ctx context.Context,
	request *protocol.GetRconProtocolsRequest,
) (*protocol.GetRconProtocolsResponse, error) {
	if p.getrconprotocols == nil {
		return &protocol.GetRconProtocolsResponse{}, nil
	}

	bytes, err := p.callFunction(ctx, p.getrconprotocols, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.GetRconProtocolsResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) GetQueryProtocols(
	ctx context.Context,
	request *protocol.GetQueryProtocolsRequest,
) (*protocol.GetQueryProtocolsResponse, error) {
	if p.getqueryprotocols == nil {
		return &protocol.GetQueryProtocolsResponse{}, nil
	}

	bytes, err := p.callFunction(ctx, p.getqueryprotocols, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.GetQueryProtocolsResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) RconOpen(
	ctx context.Context,
	request *protocol.RconOpenRequest,
) (*protocol.RconOpenResponse, error) {
	if p.rconopen == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "protocol_service_rcon_open")
	}

	bytes, err := p.callFunction(ctx, p.rconopen, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.RconOpenResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) RconExecute(
	ctx context.Context,
	request *protocol.RconExecuteRequest,
) (*protocol.RconExecuteResponse, error) {
	if p.rconexecute == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "protocol_service_rcon_execute")
	}

	bytes, err := p.callFunction(ctx, p.rconexecute, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.RconExecuteResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) RconClose(
	ctx context.Context,
	request *protocol.RconCloseRequest,
) (*protocol.RconCloseResponse, error) {
	if p.rconclose == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "protocol_service_rcon_close")
	}

	bytes, err := p.callFunction(ctx, p.rconclose, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.RconCloseResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) QueryServer(
	ctx context.Context,
	request *protocol.QueryServerRequest,
) (*protocol.QueryServerResponse, error) {
	if p.queryserver == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "protocol_service_query_server")
	}

	bytes, err := p.callFunction(ctx, p.queryserver, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.QueryServerResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) ParsePlayers(
	ctx context.Context,
	request *protocol.ParsePlayersRequest,
) (*protocol.ParsePlayersResponse, error) {
	if p.parseplayers == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "protocol_service_parse_players")
	}

	bytes, err := p.callFunction(ctx, p.parseplayers, request)
	if err != nil {
		return nil, err
	}

	response := new(protocol.ParsePlayersResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}
