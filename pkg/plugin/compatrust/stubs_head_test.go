package compatrust

import (
	"context"

	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodes"
	domainproto "github.com/gameap/gameap/pkg/proto"
)

// Host-module methods that exist only on the development panel (added after
// v4.4.2: nodefs hash and archive operations, nodes write and setup-key
// operations). The compatibility workflow deletes this file on every tagged
// matrix leg, where the stubbed interface does not have them; the fixtures
// never call these methods.

func (s *stubNodeFSService) Hash(
	_ context.Context,
	_ *nodefs.HashRequest,
) (*nodefs.HashResponse, error) {
	s.record("Hash")

	return &nodefs.HashResponse{}, nil
}

func (s *stubNodeFSService) CreateArchive(
	_ context.Context,
	_ *nodefs.CreateArchiveRequest,
) (*nodefs.ArchiveSyncResponse, error) {
	s.record("CreateArchive")

	return &nodefs.ArchiveSyncResponse{}, nil
}

func (s *stubNodeFSService) ExtractArchive(
	_ context.Context,
	_ *nodefs.ExtractArchiveRequest,
) (*nodefs.ArchiveSyncResponse, error) {
	s.record("ExtractArchive")

	return &nodefs.ArchiveSyncResponse{}, nil
}

func (s *stubNodeFSService) StartCreateArchive(
	_ context.Context,
	_ *nodefs.CreateArchiveRequest,
) (*nodefs.StartArchiveResponse, error) {
	s.record("StartCreateArchive")

	return &nodefs.StartArchiveResponse{}, nil
}

func (s *stubNodeFSService) StartExtractArchive(
	_ context.Context,
	_ *nodefs.ExtractArchiveRequest,
) (*nodefs.StartArchiveResponse, error) {
	s.record("StartExtractArchive")

	return &nodefs.StartArchiveResponse{}, nil
}

func (s *stubNodeFSService) CancelArchive(
	_ context.Context,
	_ *nodefs.CancelArchiveRequest,
) (*nodefs.CancelArchiveResponse, error) {
	s.record("CancelArchive")

	return &nodefs.CancelArchiveResponse{}, nil
}

func (s *stubNodeFSService) GetArchiveOperation(
	_ context.Context,
	_ *nodefs.GetArchiveOperationRequest,
) (*nodefs.GetArchiveOperationResponse, error) {
	s.record("GetArchiveOperation")

	return &nodefs.GetArchiveOperationResponse{}, nil
}

func (s *stubNodesService) UpdateNode(
	_ context.Context,
	req *nodes.UpdateNodeRequest,
) (*nodes.UpdateNodeResponse, error) {
	s.record("UpdateNode")

	return &nodes.UpdateNodeResponse{
		Success: true,
		Node:    &domainproto.Node{Id: req.Id, Name: "node-1"},
	}, nil
}

func (s *stubNodesService) DeleteNode(
	_ context.Context,
	_ *nodes.DeleteNodeRequest,
) (*nodes.DeleteNodeResponse, error) {
	s.record("DeleteNode")

	return &nodes.DeleteNodeResponse{Success: true}, nil
}

func (s *stubNodesService) CreateSetupKey(
	_ context.Context,
	_ *nodes.CreateSetupKeyRequest,
) (*nodes.CreateSetupKeyResponse, error) {
	s.record("CreateSetupKey")

	return &nodes.CreateSetupKeyResponse{
		Success:    true,
		SetupKey:   "setup-key",
		TicketId:   "ticket-1",
		ConnectUrl: "https://panel.example.com/enroll",
	}, nil
}

func (s *stubNodesService) GetSetupKey(
	_ context.Context,
	_ *nodes.GetSetupKeyRequest,
) (*nodes.GetSetupKeyResponse, error) {
	s.record("GetSetupKey")

	return &nodes.GetSetupKeyResponse{
		Success: true,
		Found:   true,
		Status:  nodes.SetupKeyStatus_SETUP_KEY_STATUS_PENDING,
	}, nil
}

func (s *stubNodesService) RevokeSetupKey(
	_ context.Context,
	_ *nodes.RevokeSetupKeyRequest,
) (*nodes.RevokeSetupKeyResponse, error) {
	s.record("RevokeSetupKey")

	return &nodes.RevokeSetupKeyResponse{Success: true}, nil
}
