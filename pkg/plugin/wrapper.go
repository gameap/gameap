package plugin

import (
	"context"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero/api"
)

// defaultCallTimeout caps guest calls whose caller did not set a deadline,
// so a runaway plugin cannot hold the per-plugin call gate forever.
const defaultCallTimeout = 30 * time.Second

// pluginServiceWrapper wraps WASM module calls to implement proto.PluginService.
type pluginServiceWrapper struct {
	// gate serializes guest calls; a channel instead of a mutex so queued
	// callers can abandon the wait when their context ends.
	gate                chan struct{}
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
	getserverabilities  api.Function
	getassets           api.Function
	getrconprotocols    api.Function
	getqueryprotocols   api.Function
	rconopen            api.Function
	rconexecute         api.Function
	rconclose           api.Function
	queryserver         api.Function
	parseplayers        api.Function
	handlescheduledtask api.Function

	handlearchiveprogress  api.Function
	handlearchivecompleted api.Function

	// guestLogs is flushed after every call so a partial stdout/stderr
	// line written by the guest reaches the log; nil in tests.
	guestLogs *guestLogs
	// onClosed fires when a call finds the module closed by the guest
	// itself (proc_exit); deadline closes are reported by the callers.
	onClosed func(err error)

	// observer receives the outcome and duration of every guest call;
	// pluginID labels them (0 for transient loads). Set by the manager.
	observer Observer
	pluginID uint64
}

func (p *pluginServiceWrapper) callFunction(
	ctx context.Context,
	fn api.Function,
	request vtMarshaler,
) ([]byte, error) {
	start := time.Now()

	// The wait honors the caller's full context (deadline and cancellation):
	// the guest has not been invoked yet, so giving up here is always safe.
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		p.observeGuestCall(fn, start, GuestCallResultBusy)

		return nil, errors.Wrapf(ErrPluginBusy, "%s", ctx.Err())
	}
	defer func() { <-p.gate }()
	defer p.guestLogs.Flush()

	// select picks randomly when both cases are ready.
	if ctx.Err() != nil {
		p.observeGuestCall(fn, start, GuestCallResultBusy)

		return nil, errors.Wrapf(ErrPluginBusy, "%s", ctx.Err())
	}

	// The runtime closes the module when the call context is done
	// (WithCloseOnContextDone), so caller cancellation (e.g. a client
	// dropping an HTTP request) must not reach the guest — only explicit
	// deadlines may interrupt it.
	var cancel context.CancelFunc
	if deadline, ok := ctx.Deadline(); ok {
		ctx, cancel = context.WithDeadline(context.WithoutCancel(ctx), deadline)
	} else {
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), defaultCallTimeout)
	}
	defer cancel()

	data, err := request.MarshalVT()
	if err != nil {
		return nil, err
	}

	dataSize := uint64(len(data))

	var dataPtr uint64
	if dataSize != 0 {
		results, callErr := p.malloc.Call(ctx, dataSize)
		if callErr != nil {
			p.observeCallError(callErr)

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
		p.observeCallError(err)
		p.observeGuestCall(fn, start, guestCallResult(err))

		return nil, err
	}

	resPtr := uint32(ptrSize[0] >> 32)
	resSize := uint32(ptrSize[0]) //nolint:gosec
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
		p.observeGuestCall(fn, start, GuestCallResultError)

		return nil, errors.WithMessage(ErrPluginReturnedError, string(bytes))
	}

	p.observeGuestCall(fn, start, GuestCallResultOK)

	return bytes, nil
}

// observeGuestCall reports one guest call to the observer under the
// function's export name.
func (p *pluginServiceWrapper) observeGuestCall(fn api.Function, start time.Time, result string) {
	if p.observer == nil {
		return
	}

	export := ""
	if names := fn.Definition().ExportNames(); len(names) > 0 {
		export = names[0]
	}

	p.observer.GuestCall(p.pluginID, export, time.Since(start), result)
}

// guestCallResult classifies a failed guest call for the observer.
func guestCallResult(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return GuestCallResultTimeout
	case errors.Is(err, ErrPluginBusy):
		return GuestCallResultBusy
	default:
		return GuestCallResultError
	}
}

// observeCallError reports a module the guest closed on its own. A close
// caused by the call deadline is left to the caller, which knows what the
// guest was doing and disables the plugin with that reason.
func (p *pluginServiceWrapper) observeCallError(err error) {
	if p.onClosed == nil || !p.module.IsClosed() {
		return
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return
	}

	p.onClosed(err)
}

// MemorySize reports the module's linear memory size between guest calls;
// false while a call is in flight (the memory may be growing) or once the
// module is closed.
func (p *pluginServiceWrapper) MemorySize() (uint64, bool) {
	select {
	case p.gate <- struct{}{}:
	default:
		return 0, false
	}
	defer func() { <-p.gate }()

	if p.module.IsClosed() {
		return 0, false
	}

	memory := p.module.Memory()
	if memory == nil {
		return 0, false
	}

	return uint64(memory.Size()), true
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

func (p *pluginServiceWrapper) GetServerAbilities(
	ctx context.Context,
	request *proto.GetServerAbilitiesRequest,
) (*proto.GetServerAbilitiesResponse, error) {
	if p.getserverabilities == nil {
		return &proto.GetServerAbilitiesResponse{Abilities: nil}, nil
	}

	bytes, err := p.callFunction(ctx, p.getserverabilities, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.GetServerAbilitiesResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) GetAssets(
	ctx context.Context,
	request *proto.GetAssetsRequest,
) (*proto.GetAssetsResponse, error) {
	if p.getassets == nil {
		return &proto.GetAssetsResponse{}, nil
	}

	bytes, err := p.callFunction(ctx, p.getassets, request)
	if err != nil {
		return nil, err
	}

	response := new(proto.GetAssetsResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

// HandleScheduledTask invokes the optional handler exported by plugins built
// with the sdk/scheduler module. Unlike the optional load-time queries above,
// a missing export is an error, not a benign empty response — silently
// succeeding would swallow a scheduled run.
func (p *pluginServiceWrapper) HandleScheduledTask(
	ctx context.Context,
	request *scheduler.HandleScheduledTaskRequest,
) (*scheduler.HandleScheduledTaskResponse, error) {
	if p.handlescheduledtask == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "scheduled_task_handler_handle_scheduled_task")
	}

	bytes, err := p.callFunction(ctx, p.handlescheduledtask, request)
	if err != nil {
		return nil, err
	}

	response := new(scheduler.HandleScheduledTaskResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) HasScheduledTaskHandler() bool {
	return p.handlescheduledtask != nil
}

// HandleArchiveProgress invokes the optional handler exported by plugins
// registering an ArchiveEventsHandler from the sdk/nodefs module. Like
// HandleScheduledTask, a missing export is an error — callers gate on
// HasArchiveEventsHandler and never deliver into a plugin without it.
func (p *pluginServiceWrapper) HandleArchiveProgress(
	ctx context.Context,
	request *nodefs.HandleArchiveProgressRequest,
) (*nodefs.HandleArchiveProgressResponse, error) {
	if p.handlearchiveprogress == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "archive_events_handler_handle_archive_progress")
	}

	bytes, err := p.callFunction(ctx, p.handlearchiveprogress, request)
	if err != nil {
		return nil, err
	}

	response := new(nodefs.HandleArchiveProgressResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) HandleArchiveCompleted(
	ctx context.Context,
	request *nodefs.HandleArchiveCompletedRequest,
) (*nodefs.HandleArchiveCompletedResponse, error) {
	if p.handlearchivecompleted == nil {
		return nil, errors.WithMessage(ErrExportNotFound, "archive_events_handler_handle_archive_completed")
	}

	bytes, err := p.callFunction(ctx, p.handlearchivecompleted, request)
	if err != nil {
		return nil, err
	}

	response := new(nodefs.HandleArchiveCompletedResponse)
	if err = response.UnmarshalVT(bytes); err != nil {
		return nil, err
	}

	return response, nil
}

func (p *pluginServiceWrapper) HasArchiveEventsHandler() bool {
	return p.handlearchiveprogress != nil && p.handlearchivecompleted != nil
}
