package compatrust

import (
	"context"

	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
)

// Host-module methods that exist only on the development panel (added after
// v4.4.2: nodefs hash and archive operations). The compatibility workflow
// deletes this file on every tagged matrix leg, where the stubbed interface
// does not have them; the fixtures never call these methods.

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
