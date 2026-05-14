package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

const (
	fileDispatchTimeout = 30 * time.Second
	fileTransferTimeout = 10 * time.Minute
)

type fileDispatcher struct {
	ps         pubsub.PubSub
	gateway    FileGateway
	registry   ConnectionChecker
	storage    files.StreamFileManager
	instanceID string
	logger     *slog.Logger

	mu          sync.RWMutex
	pendingReqs map[string]chan *messages.DaemonFileResponsePayload
}

func NewFileDispatcher(
	ps pubsub.PubSub,
	gateway FileGateway,
	registry ConnectionChecker,
	storage files.StreamFileManager,
	instanceID string,
	logger *slog.Logger,
) FileDispatcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &fileDispatcher{
		ps:          ps,
		gateway:     gateway,
		registry:    registry,
		storage:     storage,
		instanceID:  instanceID,
		logger:      logger,
		pendingReqs: make(map[string]chan *messages.DaemonFileResponsePayload),
	}
}

func (d *fileDispatcher) Start(ctx context.Context) error {
	if err := d.ps.Subscribe(ctx, channels.DaemonFileRequestAll, d.handleFileRequest); err != nil {
		return errors.Wrap(err, "subscribe to file request dispatch")
	}

	responseChannel := channels.BuildDaemonFileResponseChannel(d.instanceID)
	if err := d.ps.Subscribe(ctx, responseChannel, d.handleFileResponse); err != nil {
		return errors.Wrap(err, "subscribe to file response")
	}

	d.logger.Info("file dispatcher started", "instance_id", d.instanceID)

	return nil
}

func (d *fileDispatcher) dispatchAndWait(
	ctx context.Context,
	nodeID uint64,
	payload messages.DaemonFileRequestPayload,
	timeout time.Duration,
) (*messages.DaemonFileResponsePayload, error) {
	respCh := make(chan *messages.DaemonFileResponsePayload, 1)

	d.mu.Lock()
	d.pendingReqs[payload.RequestID] = respCh
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		delete(d.pendingReqs, payload.RequestID)
		d.mu.Unlock()
	}()

	channel := channels.BuildDaemonFileRequestChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonFileRequest, payload)
	if err != nil {
		return nil, errors.WithMessage(err, "create file request message")
	}

	if err := d.ps.Publish(ctx, channel, msg); err != nil {
		return nil, errors.Wrap(err, "publish file request")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, errors.New("file operation dispatch timed out")
	case resp := <-respCh:
		if resp == nil {
			return nil, errors.New("file dispatch request cancelled")
		}
		if resp.Error != "" {
			return nil, errors.New(resp.Error)
		}

		return resp, nil
	}
}

func (d *fileDispatcher) DispatchFileList(
	ctx context.Context, nodeID uint64, path string, recursive bool, pattern string,
) (*proto.FileListResponse, error) {
	req := &proto.FileListRequest{
		Path:      path,
		Recursive: recursive,
		Pattern:   pattern,
	}
	reqData, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal file list request")
	}

	resp, err := d.dispatchAndWait(ctx, nodeID, messages.DaemonFileRequestPayload{
		NodeID:     nodeID,
		RequestID:  idgen.New(),
		InstanceID: d.instanceID,
		Operation:  "file_list",
		Data:       reqData,
	}, fileDispatchTimeout)
	if err != nil {
		return nil, err
	}

	var listResp proto.FileListResponse
	if err := listResp.UnmarshalVT(resp.Data); err != nil {
		return nil, errors.Wrap(err, "unmarshal file list response")
	}

	return &listResp, nil
}

func (d *fileDispatcher) DispatchFileRead(
	ctx context.Context, nodeID uint64, path string, offset int64, length int64,
) (*FileReadResult, error) {
	req := &proto.FileReadRequest{
		Path:   path,
		Offset: offset,
		Length: length,
	}
	reqData, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal file read request")
	}

	resp, err := d.dispatchAndWait(ctx, nodeID, messages.DaemonFileRequestPayload{
		NodeID:     nodeID,
		RequestID:  idgen.New(),
		InstanceID: d.instanceID,
		Operation:  "file_read",
		Data:       reqData,
	}, fileDispatchTimeout)
	if err != nil {
		return nil, err
	}

	if resp.StoragePath != "" {
		return &FileReadResult{StoragePath: resp.StoragePath}, nil
	}

	var readResp proto.FileReadResponse
	if err := readResp.UnmarshalVT(resp.Data); err != nil {
		return nil, errors.Wrap(err, "unmarshal file read response")
	}

	if !readResp.Success {
		return nil, errors.Errorf("file read failed: %s", readResp.Error)
	}

	return &FileReadResult{Content: readResp.Content}, nil
}

func (d *fileDispatcher) DispatchFileWrite(
	ctx context.Context, nodeID uint64, path string,
	content []byte, mode int32, createDirs bool, owner OwnerOptions,
) error {
	req := &proto.FileWriteRequest{
		Path:       path,
		Content:    content,
		Mode:       mode,
		CreateDirs: createDirs,
		OwnerUser:  owner.User,
		OwnerUid:   owner.UID,
		OwnerGid:   owner.GID,
	}
	reqData, err := req.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal file write request")
	}

	_, err = d.dispatchAndWait(ctx, nodeID, messages.DaemonFileRequestPayload{
		NodeID:     nodeID,
		RequestID:  idgen.New(),
		InstanceID: d.instanceID,
		Operation:  "file_write",
		Data:       reqData,
	}, fileDispatchTimeout)

	return err
}

func (d *fileDispatcher) DispatchFileOperation(
	ctx context.Context, nodeID uint64, req *proto.FileOperationRequest,
) (*proto.FileOperationResponse, error) {
	reqData, err := req.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal file operation request")
	}

	resp, err := d.dispatchAndWait(ctx, nodeID, messages.DaemonFileRequestPayload{
		NodeID:     nodeID,
		RequestID:  idgen.New(),
		InstanceID: d.instanceID,
		Operation:  "file_operation",
		Data:       reqData,
	}, fileDispatchTimeout)
	if err != nil {
		return nil, err
	}

	var opResp proto.FileOperationResponse
	if err := opResp.UnmarshalVT(resp.Data); err != nil {
		return nil, errors.Wrap(err, "unmarshal file operation response")
	}

	return &opResp, nil
}

func (d *fileDispatcher) DispatchUploadTask(
	ctx context.Context, nodeID uint64, transferID string, destPath string,
	mode int32, owner OwnerOptions,
) error {
	_, err := d.dispatchAndWait(ctx, nodeID, messages.DaemonFileRequestPayload{
		NodeID:      nodeID,
		RequestID:   idgen.New(),
		InstanceID:  d.instanceID,
		Operation:   "upload_task",
		TransferID:  transferID,
		StoragePath: destPath,
		OwnerUser:   owner.User,
		OwnerUID:    owner.UID,
		OwnerGID:    owner.GID,
		Mode:        mode,
	}, fileTransferTimeout)

	return err
}

func (d *fileDispatcher) DispatchDownloadTask(
	ctx context.Context, nodeID uint64, transferID string, srcPath string,
) error {
	_, err := d.dispatchAndWait(ctx, nodeID, messages.DaemonFileRequestPayload{
		NodeID:      nodeID,
		RequestID:   idgen.New(),
		InstanceID:  d.instanceID,
		Operation:   "download_task",
		TransferID:  transferID,
		StoragePath: srcPath,
	}, fileTransferTimeout)

	return err
}

func (d *fileDispatcher) handleFileRequest(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonFileRequestPayload](msg)
	if err != nil {
		d.logger.Warn("failed to parse file request payload", "error", err)

		return nil
	}

	if !d.registry.IsConnected(payload.NodeID) {
		return nil
	}

	go d.executeAndRespond(payload) //nolint:gosec // intentionally outlives handler context

	return nil
}

func (d *fileDispatcher) executeAndRespond(payload messages.DaemonFileRequestPayload) {
	timeout := fileDispatchTimeout
	if payload.Operation == "upload_task" || payload.Operation == "download_task" {
		timeout = fileTransferTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp := d.executeFileRequest(ctx, payload)
	resp.RequestID = payload.RequestID

	channel := channels.BuildDaemonFileResponseChannel(payload.InstanceID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonFileResponse, resp)
	if err != nil {
		d.logger.Error("failed to create file response message", "error", err)

		return
	}

	if err := d.ps.Publish(ctx, channel, msg); err != nil {
		d.logger.Error("failed to publish file response",
			"request_id", payload.RequestID,
			"error", err,
		)
	}
}

func (d *fileDispatcher) executeFileRequest(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	var resp messages.DaemonFileResponsePayload

	switch payload.Operation {
	case "file_list":
		resp = d.execFileList(ctx, payload)
	case "file_read":
		resp = d.execFileRead(ctx, payload)
	case "file_write":
		resp = d.execFileWrite(ctx, payload)
	case "file_operation":
		resp = d.execFileOperation(ctx, payload)
	case "upload_task":
		resp = d.execUploadTask(ctx, payload)
	case "download_task":
		resp = d.execDownloadTask(ctx, payload)
	default:
		resp.Error = "unsupported operation: " + payload.Operation
	}

	return resp
}

func (d *fileDispatcher) execFileList(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	var req proto.FileListRequest
	if err := req.UnmarshalVT(payload.Data); err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	resp, err := d.gateway.RequestFileList(ctx, payload.NodeID, req.Path, req.Recursive, req.Pattern)
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	data, err := resp.MarshalVT()
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	return messages.DaemonFileResponsePayload{Data: data}
}

func (d *fileDispatcher) execFileRead(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	var req proto.FileReadRequest
	if err := req.UnmarshalVT(payload.Data); err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	resp, err := d.gateway.RequestFileRead(ctx, payload.NodeID, req.Path, req.Offset, req.Length)
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	data, err := resp.MarshalVT()
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	return messages.DaemonFileResponsePayload{Data: data}
}

func (d *fileDispatcher) execFileWrite(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	var req proto.FileWriteRequest
	if err := req.UnmarshalVT(payload.Data); err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	err := d.gateway.RequestFileWrite(
		ctx, payload.NodeID, req.Path, req.Content, req.Mode, req.CreateDirs,
		OwnerOptions{User: req.OwnerUser, UID: req.OwnerUid, GID: req.OwnerGid},
	)
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	return messages.DaemonFileResponsePayload{}
}

func (d *fileDispatcher) execFileOperation(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	var req proto.FileOperationRequest
	if err := req.UnmarshalVT(payload.Data); err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	resp, err := d.gateway.RequestFileOperation(ctx, payload.NodeID, &req)
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	data, err := resp.MarshalVT()
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	return messages.DaemonFileResponsePayload{Data: data}
}

func (d *fileDispatcher) execUploadTask(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	err := d.gateway.RequestFileUploadTask(
		ctx, payload.NodeID, payload.TransferID, payload.StoragePath, "", 0, payload.Mode,
		OwnerOptions{User: payload.OwnerUser, UID: payload.OwnerUID, GID: payload.OwnerGID},
	)
	if err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	_ = d.storage.Delete(context.Background(), transferPrefix+payload.TransferID+"/data")

	return messages.DaemonFileResponsePayload{}
}

func (d *fileDispatcher) execDownloadTask(
	ctx context.Context,
	payload messages.DaemonFileRequestPayload,
) messages.DaemonFileResponsePayload {
	if err := d.gateway.RequestFileDownloadTask(
		ctx, payload.NodeID, payload.TransferID, payload.StoragePath,
	); err != nil {
		return messages.DaemonFileResponsePayload{Error: err.Error()}
	}

	return messages.DaemonFileResponsePayload{
		StoragePath: transferPrefix + payload.TransferID + "/data",
	}
}

func (d *fileDispatcher) handleFileResponse(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonFileResponsePayload](msg)
	if err != nil {
		d.logger.Warn("failed to parse file response payload", "error", err)

		return nil
	}

	d.mu.RLock()
	ch, ok := d.pendingReqs[payload.RequestID]
	d.mu.RUnlock()

	if ok {
		select {
		case ch <- &payload:
		default:
		}
	}

	return nil
}
