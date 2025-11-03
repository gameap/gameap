package binnapi

import (
	"os"

	"github.com/et-nik/binngo"
	"github.com/et-nik/binngo/decode"
	"github.com/pkg/errors"
)

type FilesOperation uint8

const (
	FilesOperationFileSend   FilesOperation = 3
	FilesOperationReadDir    FilesOperation = 4
	FilesOperationMakeDir    FilesOperation = 5
	FilesOperationFileMove   FilesOperation = 6
	FilesOperationFileRemove FilesOperation = 7
	FilesOperationFileInfo   FilesOperation = 8
	FilesOperationFileChmod  FilesOperation = 9
)

const (
	FilesListWithoutDetails = 0
	FilesListWithDetails    = 1
)

const (
	FilesGetFileFromClient = 1
	FilesSendFileToClient  = 2
)

// FileType represents the type of file system entry.
type FileType uint8

const (
	TypeUnknown     FileType = 0
	TypeDir         FileType = 1
	TypeFile        FileType = 2
	TypeCharDevice  FileType = 3
	TypeBlockDevice FileType = 4
	TypeNamedPipe   FileType = 5
	TypeSymlink     FileType = 6
	TypeSocket      FileType = 7
)

type FileSize uint64

func CreateFileSize(size any) (FileSize, error) {
	result, err := convertToUint64(size)
	if err != nil {
		return 0, errors.WithMessage(err, "invalid file size")
	}

	return FileSize(result), nil
}

// Helper type conversion functions.
func convertToCode(val any) (uint8, error) {
	switch v := val.(type) {
	case uint8:
		return v, nil
	case int8:
		if v < 0 {
			return 0, NewInvalidBINNValueError("cannot convert negative int8 to uint8 code")
		}

		return uint8(v), nil
	default:
		return 0, NewInvalidBINNValueError("cannot convert to uint8 code")
	}
}

func convertToUint64(val any) (uint64, error) {
	switch v := val.(type) {
	case uint:
		return uint64(v), nil
	case int:
		if v < 0 {
			return 0, NewInvalidBINNValueError("cannot convert negative int to uint64")
		}

		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case int8:
		if v < 0 {
			return 0, NewInvalidBINNValueError("cannot convert negative int to uint64")
		}

		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case int16:
		if v < 0 {
			return 0, NewInvalidBINNValueError("cannot convert negative int to uint64")
		}

		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case int32:
		if v < 0 {
			return 0, NewInvalidBINNValueError("cannot convert negative int to uint64")
		}

		return uint64(v), nil
	case uint64:
		return v, nil
	case int64:
		if v < 0 {
			return 0, NewInvalidBINNValueError("cannot convert negative int to uint64")
		}

		return uint64(v), nil
	default:
		return 0, NewInvalidBINNValueError("cannot convert to uint64")
	}
}

// fileTypeByMode determines file type from os.FileMode.
func fileTypeByMode(fileMode os.FileMode) FileType {
	fType := TypeUnknown

	switch {
	case fileMode&os.ModeSymlink != 0:
		fType = TypeSymlink
	case fileMode.IsRegular():
		fType = TypeFile
	case fileMode.IsDir():
		fType = TypeDir
	case fileMode&os.ModeCharDevice != 0:
		fType = TypeCharDevice
	case fileMode&os.ModeDevice != 0:
		fType = TypeBlockDevice
	case fileMode&os.ModeNamedPipe != 0:
		fType = TypeNamedPipe
	case fileMode&os.ModeSocket != 0:
		fType = TypeSocket
	}

	return fType
}

// Request messages

// ReadDirRequestMessage represents a request to read directory contents.
type ReadDirRequestMessage struct {
	Directory   string
	DetailsMode bool
}

// UnmarshalBINN deserializes a ReadDirRequestMessage from BINN format.
func (r *ReadDirRequestMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 3 {
		return NewInvalidBINNValueError("read dir message requires at least 3 fields")
	}

	directory, ok := m[1].(string)
	if !ok {
		return NewInvalidBINNValueError("directory must be string")
	}

	detailsMode, err := convertToCode(m[2])
	if err != nil {
		return err
	}

	r.Directory = directory
	r.DetailsMode = detailsMode != 0

	return nil
}

// MarshalBINN serializes a ReadDirRequestMessage to BINN format.
func (r *ReadDirRequestMessage) MarshalBINN() ([]byte, error) {
	detailsMode := uint8(FilesListWithoutDetails)
	if r.DetailsMode {
		detailsMode = FilesListWithDetails
	}

	resp := []any{FilesOperationReadDir, r.Directory, detailsMode}

	return binngo.Marshal(&resp)
}

// MkDirRequestMessage represents a request to create a directory.
type MkDirRequestMessage struct {
	Directory string
}

// UnmarshalBINN deserializes a MkDirRequestMessage from BINN format.
func (m *MkDirRequestMessage) UnmarshalBINN(bytes []byte) error {
	var v []any

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return err
	}

	if len(v) < 2 {
		return NewInvalidBINNValueError("mkdir message requires at least 2 fields")
	}

	directory, ok := v[1].(string)
	if !ok {
		return NewInvalidBINNValueError("directory must be string")
	}

	m.Directory = directory

	return nil
}

// MarshalBINN serializes a MkDirRequestMessage to BINN format.
func (m *MkDirRequestMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationMakeDir, m.Directory}

	return binngo.Marshal(&resp)
}

// MoveRequestMessage represents a request to move or copy a file.
type MoveRequestMessage struct {
	Source      string
	Destination string
	Copy        bool
}

// UnmarshalBINN deserializes a MoveRequestMessage from BINN format.
func (m *MoveRequestMessage) UnmarshalBINN(bytes []byte) error {
	var v []any

	err := decode.Unmarshal(bytes, &v)
	if err != nil {
		return err
	}

	if len(v) < 4 {
		return NewInvalidBINNValueError("move message requires at least 4 fields")
	}

	source, ok := v[1].(string)
	if !ok {
		return NewInvalidBINNValueError("source must be string")
	}

	destination, ok := v[2].(string)
	if !ok {
		return NewInvalidBINNValueError("destination must be string")
	}

	cp, ok := v[3].(bool)
	if !ok {
		return NewInvalidBINNValueError("copy flag must be bool")
	}

	m.Source = source
	m.Destination = destination
	m.Copy = cp

	return nil
}

// MarshalBINN serializes a MoveRequestMessage to BINN format.
func (m *MoveRequestMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationFileMove, m.Source, m.Destination, m.Copy}

	return binngo.Marshal(&resp)
}

// DownloadRequestMessage represents a request to send a file to the client.
type DownloadRequestMessage struct {
	FilePath string
}

// UnmarshalBINN deserializes a DownloadRequestMessage from BINN format.
func (s *DownloadRequestMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 3 {
		return NewInvalidBINNValueError("send file message requires at least 3 fields")
	}

	filePath, ok := m[2].(string)
	if !ok {
		return NewInvalidBINNValueError("file path must be string")
	}

	s.FilePath = filePath

	return nil
}

// MarshalBINN serializes a DownloadRequestMessage to BINN format.
func (s *DownloadRequestMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationFileSend, FilesSendFileToClient, s.FilePath}

	return binngo.Marshal(&resp)
}

// UploadRequestMessage represents a request to receive a file from the client.
type UploadRequestMessage struct {
	FilePath string
	FileSize uint64
	MakeDirs bool
	Perms    os.FileMode
}

// UnmarshalBINN deserializes a UploadRequestMessage from BINN format.
func (g *UploadRequestMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 6 {
		return NewInvalidBINNValueError("get file message requires at least 6 fields")
	}

	filePath, ok := m[2].(string)
	if !ok {
		return NewInvalidBINNValueError("file path must be string")
	}

	fileSize, err := convertToUint64(m[3])
	if err != nil {
		return NewInvalidBINNValueError("invalid file size")
	}

	makeDirs, ok := m[4].(bool)
	if !ok {
		return NewInvalidBINNValueError("make dirs flag must be bool")
	}

	perms, err := convertToUint64(m[5])
	if err != nil {
		return NewInvalidBINNValueError("invalid permissions")
	}

	if perms > 0xFFFFFFFF {
		return NewInvalidBINNValueError("permissions value too large")
	}

	g.FilePath = filePath
	g.FileSize = fileSize
	g.MakeDirs = makeDirs
	g.Perms = os.FileMode(perms)

	return nil
}

// MarshalBINN serializes a UploadRequestMessage to BINN format.
func (g *UploadRequestMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationFileSend, FilesGetFileFromClient, g.FilePath, g.FileSize, g.MakeDirs, uint64(g.Perms)}

	return binngo.Marshal(&resp)
}

// RemoveRequestMessage represents a request to remove a file or directory.
type RemoveRequestMessage struct {
	Path      string
	Recursive bool
}

// UnmarshalBINN deserializes a RemoveRequestMessage from BINN format.
func (r *RemoveRequestMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 3 {
		return NewInvalidBINNValueError("remove message requires at least 3 fields")
	}

	path, ok := m[1].(string)
	if !ok {
		return NewInvalidBINNValueError("path must be string")
	}

	recursive, ok := m[2].(bool)
	if !ok {
		return NewInvalidBINNValueError("recursive flag must be bool")
	}

	r.Path = path
	r.Recursive = recursive

	return nil
}

// MarshalBINN serializes a RemoveRequestMessage to BINN format.
func (r *RemoveRequestMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationFileRemove, r.Path, r.Recursive}

	return binngo.Marshal(&resp)
}

// FileInfoRequestMessage represents a request for file information.
type FileInfoRequestMessage struct {
	Path string
}

// UnmarshalBINN deserializes a FileInfoRequestMessage from BINN format.
func (f *FileInfoRequestMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 2 {
		return NewInvalidBINNValueError("file info message requires at least 2 fields")
	}

	path, ok := m[1].(string)
	if !ok {
		return NewInvalidBINNValueError("path must be string")
	}

	f.Path = path

	return nil
}

// MarshalBINN serializes a FileInfoRequestMessage to BINN format.
func (f *FileInfoRequestMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationFileInfo, f.Path}

	return binngo.Marshal(&resp)
}

// ChmodMessage represents a request to change file permissions.
type ChmodMessage struct {
	Path string
	Perm uint32
}

// UnmarshalBINN deserializes a ChmodMessage from BINN format.
func (c *ChmodMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	if len(m) < 3 {
		return NewInvalidBINNValueError("chmod message requires at least 3 fields")
	}

	path, ok := m[1].(string)
	if !ok {
		return NewInvalidBINNValueError("path must be string")
	}

	perm, err := convertToUint64(m[2])
	if err != nil {
		return err
	}

	if perm > 0xFFFFFFFF {
		return NewInvalidBINNValueError("permission value too large")
	}

	c.Path = path
	c.Perm = uint32(perm)

	return nil
}

// MarshalBINN serializes a ChmodMessage to BINN format.
func (c *ChmodMessage) MarshalBINN() ([]byte, error) {
	resp := []any{FilesOperationFileChmod, c.Path, c.Perm}

	return binngo.Marshal(&resp)
}

// Response messages

// FileInfoResponseMessage represents basic file information response.
type FileInfoResponseMessage struct {
	Name         string
	Size         uint64
	TimeModified uint64
	Type         uint8
	Perm         uint32
}

// CreateFileInfoResponseMessage creates a FileInfoResponseMessage from os.FileInfo.
func CreateFileInfoResponseMessageFromFileInfo(fi os.FileInfo) *FileInfoResponseMessage {
	fType := fileTypeByMode(fi.Mode())

	size := max(fi.Size(), 0)

	modTime := max(fi.ModTime().Unix(), 0)

	return &FileInfoResponseMessage{
		Name:         fi.Name(),
		Size:         uint64(size),    // #nosec G115 -- validated non-negative
		TimeModified: uint64(modTime), // #nosec G115 -- validated non-negative
		Type:         uint8(fType),
		Perm:         uint32(fi.Mode().Perm()),
	}
}

// parseFileInfoResponseMessageFromArray extracts file info from a BINN array.
func parseFileInfoResponseMessageFromArray(m []any) (*FileInfoResponseMessage, error) {
	if len(m) < 5 {
		return nil, NewInvalidBINNValueError("file info response requires at least 5 fields")
	}

	fi := &FileInfoResponseMessage{}

	name, ok := m[0].(string)
	if !ok {
		return nil, NewInvalidBINNValueError("name must be string")
	}
	fi.Name = name

	size, err := convertToUint64(m[1])
	if err != nil {
		return nil, errors.WithMessage(err, "invalid size")
	}
	fi.Size = size

	timeModified, err := convertToUint64(m[2])
	if err != nil {
		return nil, NewInvalidBINNValueError("invalid time modified")
	}
	fi.TimeModified = timeModified

	fileType, err := convertToCode(m[3])
	if err != nil {
		return nil, err
	}
	fi.Type = fileType

	perm, err := convertToUint64(m[4])
	if err != nil {
		return nil, NewInvalidBINNValueError("invalid permissions")
	}

	if perm > 0xFFFFFFFF {
		return nil, NewInvalidBINNValueError("permissions value too large")
	}
	fi.Perm = uint32(perm)

	return fi, nil
}

// UnmarshalBINN deserializes a FileInfoResponseMessage from BINN format.
func (fi *FileInfoResponseMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	parsed, err := parseFileInfoResponseMessageFromArray(m)
	if err != nil {
		return err
	}

	*fi = *parsed

	return nil
}

// CreateFileInfoResponseMessage creates a FileInfoResponseMessage from BINN data.
func CreateFileInfoResponseMessage(data any) (*FileInfoResponseMessage, error) {
	m, ok := data.([]any)
	if !ok {
		return nil, NewInvalidBINNValueError("file info response data must be array")
	}

	return parseFileInfoResponseMessageFromArray(m)
}

// MarshalBINN serializes the file info response to BINN format.
func (fi FileInfoResponseMessage) MarshalBINN() ([]byte, error) {
	resp := []any{fi.Name, fi.Size, fi.TimeModified, fi.Type, fi.Perm}

	return binngo.Marshal(&resp)
}

// FileDetailsResponseMessage represents detailed file information response.
type FileDetailsResponseMessage struct {
	Name             string
	Mime             string
	Size             uint64
	ModificationTime uint64
	AccessTime       uint64
	CreateTime       uint64
	Perm             uint32
	Type             uint8
}

// parseFileDetailsResponseMessageFromArray extracts file details from a BINN array.
func parseFileDetailsResponseMessageFromArray(m []any) (*FileDetailsResponseMessage, error) {
	if len(m) < 8 {
		return nil, NewInvalidBINNValueError("file details response requires at least 8 fields")
	}

	fdr := &FileDetailsResponseMessage{}

	name, ok := m[0].(string)
	if !ok {
		return nil, NewInvalidBINNValueError("name must be string")
	}
	fdr.Name = name

	size, err := convertToUint64(m[1])
	if err != nil {
		return nil, errors.WithMessage(err, "invalid size")
	}
	fdr.Size = size

	fileType, err := convertToCode(m[2])
	if err != nil {
		return nil, err
	}
	fdr.Type = fileType

	modificationTime, err := convertToUint64(m[3])
	if err != nil {
		return nil, NewInvalidBINNValueError("invalid modification time")
	}
	fdr.ModificationTime = modificationTime

	accessTime, err := convertToUint64(m[4])
	if err != nil {
		return nil, NewInvalidBINNValueError("invalid access time")
	}
	fdr.AccessTime = accessTime

	createTime, err := convertToUint64(m[5])
	if err != nil {
		return nil, NewInvalidBINNValueError("invalid create time")
	}
	fdr.CreateTime = createTime

	perm, err := convertToUint64(m[6])
	if err != nil {
		return nil, NewInvalidBINNValueError("invalid permissions")
	}

	if perm > 0xFFFFFFFF {
		return nil, NewInvalidBINNValueError("permissions value too large")
	}
	fdr.Perm = uint32(perm)

	mime, ok := m[7].(string)
	if !ok {
		return nil, NewInvalidBINNValueError("mime must be string")
	}
	fdr.Mime = mime

	return fdr, nil
}

// UnmarshalBINN deserializes a FileDetailsResponseMessage from BINN format.
func (fdr *FileDetailsResponseMessage) UnmarshalBINN(bytes []byte) error {
	var m []any

	err := decode.Unmarshal(bytes, &m)
	if err != nil {
		return err
	}

	parsed, err := parseFileDetailsResponseMessageFromArray(m)
	if err != nil {
		return err
	}

	*fdr = *parsed

	return nil
}

// MarshalBINN serializes the file details response to BINN format.
func (fdr FileDetailsResponseMessage) MarshalBINN() ([]byte, error) {
	resp := []any{
		fdr.Name,
		fdr.Size,
		fdr.Type,
		fdr.ModificationTime,
		fdr.AccessTime,
		fdr.CreateTime,
		fdr.Perm,
		fdr.Mime,
	}

	return binngo.Marshal(&resp)
}

// CreateFileDetailsResponseMessage creates a FileDetailsResponseMessage from BINN data.
func CreateFileDetailsResponseMessage(data any) (*FileDetailsResponseMessage, error) {
	m, ok := data.([]any)
	if !ok {
		return nil, NewInvalidBINNValueError("file details response data must be array")
	}

	return parseFileDetailsResponseMessageFromArray(m)
}
