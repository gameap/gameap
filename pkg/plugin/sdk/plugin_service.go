package sdk

import (
	"context"
	"errors"

	"github.com/gameap/gameap/pkg/plugin/proto"
)

// EmptyPluginService provides an empty implementation of the PluginService interface.
// Plugins can embed this struct to inherit default empty implementations
// and only override the methods they need.
type EmptyPluginService struct{}

func (EmptyPluginService) GetInfo(context.Context, *proto.GetInfoRequest) (*proto.PluginInfo, error) {
	return nil, errors.New("not implemented") //nolint:err113
}

func (EmptyPluginService) Initialize(context.Context, *proto.InitializeRequest) (*proto.InitializeResponse, error) {
	return &proto.InitializeResponse{
		Result: &proto.Result{Success: true},
	}, nil
}

func (EmptyPluginService) Shutdown(context.Context, *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	return &proto.ShutdownResponse{
		Result: &proto.Result{Success: true},
	}, nil
}

func (EmptyPluginService) HandleEvent(context.Context, *proto.Event) (*proto.EventResult, error) {
	return &proto.EventResult{Handled: false}, nil
}

func (EmptyPluginService) GetSubscribedEvents(
	context.Context,
	*proto.GetSubscribedEventsRequest,
) (*proto.GetSubscribedEventsResponse, error) {
	return &proto.GetSubscribedEventsResponse{}, nil
}

func (EmptyPluginService) GetHTTPRoutes(
	context.Context,
	*proto.GetHTTPRoutesRequest,
) (*proto.GetHTTPRoutesResponse, error) {
	return &proto.GetHTTPRoutesResponse{}, nil
}

func (EmptyPluginService) HandleHTTPRequest(context.Context, *proto.HTTPRequest) (*proto.HTTPResponse, error) {
	return &proto.HTTPResponse{StatusCode: 404}, nil
}

func (EmptyPluginService) GetFrontendBundle(
	context.Context,
	*proto.GetFrontendBundleRequest,
) (*proto.GetFrontendBundleResponse, error) {
	return &proto.GetFrontendBundleResponse{}, nil
}

func (EmptyPluginService) GetServerAbilities(
	context.Context,
	*proto.GetServerAbilitiesRequest,
) (*proto.GetServerAbilitiesResponse, error) {
	return &proto.GetServerAbilitiesResponse{}, nil
}

func (EmptyPluginService) GetAssets(
	context.Context,
	*proto.GetAssetsRequest,
) (*proto.GetAssetsResponse, error) {
	return &proto.GetAssetsResponse{}, nil
}
