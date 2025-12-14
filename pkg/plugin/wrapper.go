package plugin

import (
	"context"
	"sync"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero/api"
)

// pluginServiceWrapper wraps WASM module calls to implement proto.PluginService.
type pluginServiceWrapper struct {
	mu                  sync.Mutex
	module              api.Module
	malloc              api.Function
	free                api.Function
	getinfo             api.Function
	initialize          api.Function
	shutdown            api.Function
	handleevent         api.Function
	getsubscribedevents api.Function
	gethttproutes       api.Function
	handlehttprequest   api.Function
	getfrontendbundle   api.Function
}

func (p *pluginServiceWrapper) callFunction(
	ctx context.Context,
	fn api.Function,
	request vtMarshaler,
) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := request.MarshalVT()
	if err != nil {
		return nil, err
	}

	dataSize := uint64(len(data))

	var dataPtr uint64
	if dataSize != 0 {
		results, callErr := p.malloc.Call(ctx, dataSize)
		if callErr != nil {
			return nil, callErr
		}

		dataPtr = results[0]
		defer p.free.Call(ctx, dataPtr) //nolint:errcheck

		if !p.module.Memory().Write(uint32(dataPtr), data) { //nolint:gosec
			return nil, errors.Wrapf(ErrMemoryOutOfRange, "write(%d, %d), size=%d",
				dataPtr, dataSize, p.module.Memory().Size())
		}
	}

	ptrSize, err := fn.Call(ctx, dataPtr, dataSize)
	if err != nil {
		return nil, err
	}

	resPtr := uint32(ptrSize[0] >> 32) //nolint:gosec
	resSize := uint32(ptrSize[0])      //nolint:gosec
	isErrResponse := (resSize & (1 << 31)) > 0

	if isErrResponse {
		resSize &^= (1 << 31)
	}

	if resPtr != 0 {
		defer p.free.Call(ctx, uint64(resPtr)) //nolint:errcheck
	}

	bytes, ok := p.module.Memory().Read(resPtr, resSize)
	if !ok {
		return nil, errors.Wrapf(ErrMemoryOutOfRange, "read(%d, %d), size=%d",
			resPtr, resSize, p.module.Memory().Size())
	}

	if isErrResponse {
		return nil, errors.WithMessage(ErrPluginReturnedError, string(bytes))
	}

	return bytes, nil
}

type vtMarshaler interface {
	MarshalVT() ([]byte, error)
}

func (p *pluginServiceWrapper) GetInfo(
	ctx context.Context,
	request *proto.GetInfoRequest,
) (*proto.PluginInfo, error) {
	bytes, err := p.callFunction(ctx, p.getinfo, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.PluginInfo)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) Initialize(
	ctx context.Context,
	request *proto.InitializeRequest,
) (*proto.InitializeResponse, error) {
	bytes, err := p.callFunction(ctx, p.initialize, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.InitializeResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) Shutdown(
	ctx context.Context,
	request *proto.ShutdownRequest,
) (*proto.ShutdownResponse, error) {
	bytes, err := p.callFunction(ctx, p.shutdown, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.ShutdownResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) HandleEvent(
	ctx context.Context,
	request *proto.Event,
) (*proto.EventResult, error) {
	bytes, err := p.callFunction(ctx, p.handleevent, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.EventResult)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) GetSubscribedEvents(
	ctx context.Context,
	request *proto.GetSubscribedEventsRequest,
) (*proto.GetSubscribedEventsResponse, error) {
	bytes, err := p.callFunction(ctx, p.getsubscribedevents, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.GetSubscribedEventsResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) GetHTTPRoutes(
	ctx context.Context,
	request *proto.GetHTTPRoutesRequest,
) (*proto.GetHTTPRoutesResponse, error) {
	bytes, err := p.callFunction(ctx, p.gethttproutes, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.GetHTTPRoutesResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) HandleHTTPRequest(
	ctx context.Context,
	request *proto.HTTPRequest,
) (*proto.HTTPResponse, error) {
	bytes, err := p.callFunction(ctx, p.handlehttprequest, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.HTTPResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) GetFrontendBundle(
	ctx context.Context,
	request *proto.GetFrontendBundleRequest,
) (*proto.GetFrontendBundleResponse, error) {
	if p.getfrontendbundle == nil {
		return &proto.GetFrontendBundleResponse{HasBundle: false}, nil
	}

	bytes, err := p.callFunction(ctx, p.getfrontendbundle, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.GetFrontendBundleResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}
