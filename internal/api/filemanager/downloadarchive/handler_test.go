package downloadarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/filemanager/archiver"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errDaemonUnavailable = errors.New("daemon unavailable")
	errBuildManifestXyz  = errors.New("xyz")
	errNodeRepoFindAll   = errors.New("findall unused")
	errNodeRepoSave      = errors.New("save unused")
	errNodeRepoDelete    = errors.New("delete unused")
	errNodeRepoDBDown    = errors.New("db down")
)

var testUser = domain.User{
	ID:    1,
	Login: "tester",
	Email: "t@example.com",
}

var testNode = domain.Node{
	ID:                  1,
	Enabled:             true,
	Name:                "node",
	OS:                  "linux",
	Location:            "loc",
	GdaemonHost:         "127.0.0.1",
	GdaemonPort:         31717,
	GdaemonAPIKey:       "k",
	WorkPath:            "/srv",
	GdaemonServerCert:   "c",
	ClientCertificateID: 1,
}

func authedCtx() context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		Login: testUser.Login,
		Email: testUser.Email,
		User:  &testUser,
	})
}

func saveServerWithFilesAbility(
	t *testing.T,
	serverRepo *inmemory.ServerRepository,
	nodeRepo *inmemory.NodeRepository,
	rbacRepo *inmemory.RBACRepository,
) {
	t.Helper()

	now := time.Now()
	server := &domain.Server{
		ID:        1,
		UID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort: "short1",
		Enabled:   true,
		Installed: 1,
		Name:      "S1",
		GameID:    "g",
		DSID:      1,
		GameModID: 1,
		ServerIP:  "127.0.0.1",
		Dir:       "servers/s1",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(testUser.ID, server.ID)

	ability := &domain.Ability{
		Name:       domain.AbilityNameGameServerFiles,
		EntityType: lo.ToPtr(domain.EntityTypeServer),
		EntityID:   new(server.ID),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))
	require.NoError(t, rbacRepo.SavePermission(context.Background(), &domain.Permission{
		AbilityID:  ability.ID,
		EntityID:   new(testUser.ID),
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}))

	node := testNode
	require.NoError(t, nodeRepo.Save(context.Background(), &node))
}

type stubArchiver struct {
	manifest      *archiver.Manifest
	manifestErr   error
	writeErr      error
	writeContent  []byte
	writeRecorded *archiver.Manifest
	recordedOpts  archiver.Options
}

func (s *stubArchiver) BuildManifest(
	_ context.Context, _ *domain.Node, _ string, _ archiver.Limits,
) (*archiver.Manifest, error) {
	return s.manifest, s.manifestErr
}

func (s *stubArchiver) WriteArchive(
	_ context.Context, w io.Writer, _ *domain.Node, m *archiver.Manifest, opts archiver.Options,
) (*archiver.Result, error) {
	s.recordedOpts = opts
	s.writeRecorded = m
	if s.writeErr != nil {
		return nil, s.writeErr
	}
	if len(s.writeContent) > 0 {
		n, err := w.Write(s.writeContent)
		if err != nil {
			return nil, err
		}

		return &archiver.Result{BytesWritten: uint64(n)}, nil
	}

	zw := zip.NewWriter(w)
	fw, _ := zw.Create("data/a.txt")
	_, _ = fw.Write([]byte("alpha"))
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return &archiver.Result{BytesWritten: 0}, nil
}

type fakeGuard struct {
	acquireErr error
	released   bool
}

func (f *fakeGuard) Acquire(_ context.Context, _ uint) (func(), error) {
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}

	return func() { f.released = true }, nil
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		serverID       string
		queryDisk      string
		queryPath      string
		queryCompress  string
		setupCtx       func() context.Context
		setupRepo      func(*testing.T, *inmemory.ServerRepository, *inmemory.NodeRepository, *inmemory.RBACRepository)
		setupArchiver  func() *stubArchiver
		setupGuard     func() *fakeGuard
		limits         Limits
		expectedStatus int
		wantError      string
		validate       func(*testing.T, *httptest.ResponseRecorder, *stubArchiver, *fakeGuard)
	}{
		{
			name:      "success_streams_zip_with_headers",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifest: &archiver.Manifest{
						RootName:   "data",
						TotalSize:  500,
						TotalFiles: 2,
						Skipped:    []string{"data/socket"},
						Entries: []archiver.Entry{
							{RelPath: "data/a.txt", Size: 250},
							{RelPath: "data/b.txt", Size: 250},
						},
					},
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, w *httptest.ResponseRecorder, _ *stubArchiver, g *fakeGuard) {
				t.Helper()
				assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
				assert.Contains(t, w.Header().Get("Content-Disposition"), "data.zip")
				assert.Equal(t, "500", w.Header().Get("X-Archive-Total-Bytes"))
				assert.Equal(t, "2", w.Header().Get("X-Archive-Total-Files"))
				assert.Equal(t, "1", w.Header().Get("X-Archive-Skipped-Count"))
				assert.True(t, g.released, "concurrency slot must be released")

				zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
				require.NoError(t, err)
				require.Len(t, zr.File, 1)
				assert.Equal(t, "data/a.txt", zr.File[0].Name)
			},
		},
		{
			name:      "error_unauthenticated",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  context.Background,
			setupRepo: func(_ *testing.T, _ *inmemory.ServerRepository, _ *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
			},
			setupArchiver:  func() *stubArchiver { return &stubArchiver{} },
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:      "error_disk_required",
			serverID:  "1",
			queryDisk: "",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "disk parameter is required",
		},
		{
			name:      "error_unsupported_disk",
			serverID:  "1",
			queryDisk: "local",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "unsupported disk",
		},
		{
			name:      "error_path_required",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path parameter is required",
		},
		{
			name:      "error_path_traversal",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "../../etc/passwd",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "path contains invalid directory traversal",
		},
		{
			name:          "error_invalid_compress",
			serverID:      "1",
			queryDisk:     "server",
			queryPath:     "data",
			queryCompress: "12",
			setupCtx:      authedCtx,
			setupRepo:     saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "compress must be an integer between 0 and 9",
		},
		{
			name:      "error_total_size_too_large",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifestErr: archiver.ErrTooLarge,
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusRequestEntityTooLarge,
			wantError:      "archive total size exceeds limit",
		},
		{
			name:      "error_too_many_files",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifestErr: archiver.ErrTooManyFiles,
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusRequestEntityTooLarge,
			wantError:      "archive total file count exceeds limit",
		},
		{
			name:      "error_empty_directory_returns_404",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifestErr: archiver.ErrEmptyManifest,
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusNotFound,
			wantError:      "nothing to archive",
		},
		{
			name:      "error_path_is_not_directory",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "file.txt",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifestErr: archiver.ErrNotADirectory,
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "requested path is not a directory",
		},
		{
			name:      "error_too_many_concurrent_returns_429",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifest: &archiver.Manifest{
						RootName:   "data",
						TotalSize:  10,
						TotalFiles: 1,
						Entries: []archiver.Entry{
							{RelPath: "data/a.txt"},
						},
					},
				}
			},
			setupGuard: func() *fakeGuard {
				return &fakeGuard{acquireErr: archiver.ErrTooManyConcurrent}
			},
			expectedStatus: http.StatusTooManyRequests,
			wantError:      "too many concurrent archive downloads",
		},
		{
			name:      "error_user_without_files_permission",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: func(t *testing.T, serverRepo *inmemory.ServerRepository, nodeRepo *inmemory.NodeRepository, _ *inmemory.RBACRepository) {
				t.Helper()
				now := time.Now()
				server := &domain.Server{
					ID: 1, UID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short", Enabled: true, Installed: 1, Name: "S1",
					GameID: "g", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1",
					Dir: "servers/s1", CreatedAt: &now, UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser.ID, server.ID)

				node := testNode
				require.NoError(t, nodeRepo.Save(context.Background(), &node))
			},
			setupArchiver:  func() *stubArchiver { return &stubArchiver{} },
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusForbidden,
			wantError:      "user does not have required permissions",
		},
		{
			name:      "error_archiver_internal_failure",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifestErr: errDaemonUnavailable,
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:          "success_compress_zero_produces_store_zip",
			serverID:      "1",
			queryDisk:     "server",
			queryPath:     "data",
			queryCompress: "0",
			setupCtx:      authedCtx,
			setupRepo:     saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifest: &archiver.Manifest{
						RootName: "data",
						Entries:  []archiver.Entry{{RelPath: "data/a.txt"}},
					},
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, _ *httptest.ResponseRecorder, arch *stubArchiver, _ *fakeGuard) {
				t.Helper()
				assert.Equal(t, 0, arch.recordedOpts.CompressLevel, "explicit zero must reach archiver as 0")
			},
		},
		{
			name:          "success_compress_max_level_nine",
			serverID:      "1",
			queryDisk:     "server",
			queryPath:     "data",
			queryCompress: "9",
			setupCtx:      authedCtx,
			setupRepo:     saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifest: &archiver.Manifest{
						RootName: "data",
						Entries:  []archiver.Entry{{RelPath: "data/a.txt"}},
					},
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, _ *httptest.ResponseRecorder, arch *stubArchiver, _ *fakeGuard) {
				t.Helper()
				assert.Equal(t, 9, arch.recordedOpts.CompressLevel, "level 9 must reach archiver unchanged")
			},
		},
		{
			name:          "success_compress_whitespace_trimmed",
			serverID:      "1",
			queryDisk:     "server",
			queryPath:     "data",
			queryCompress: "  3  ",
			setupCtx:      authedCtx,
			setupRepo:     saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifest: &archiver.Manifest{
						RootName: "data",
						Entries:  []archiver.Entry{{RelPath: "data/a.txt"}},
					},
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, _ *httptest.ResponseRecorder, arch *stubArchiver, _ *fakeGuard) {
				t.Helper()
				assert.Equal(t, 3, arch.recordedOpts.CompressLevel, "whitespace must be trimmed before parsing")
			},
		},
		{
			name:      "success_cache_control_header_set",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifest: &archiver.Manifest{
						RootName: "data",
						Entries:  []archiver.Entry{{RelPath: "data/a.txt"}},
					},
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, w *httptest.ResponseRecorder, _ *stubArchiver, _ *fakeGuard) {
				t.Helper()
				assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), "Cache-Control must be no-store")
			},
		},
		{
			name:           "error_compress_negative",
			serverID:       "1",
			queryDisk:      "server",
			queryPath:      "data",
			queryCompress:  "-1",
			setupCtx:       authedCtx,
			setupRepo:      saveServerWithFilesAbility,
			setupArchiver:  func() *stubArchiver { return &stubArchiver{} },
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "compress must be",
		},
		{
			name:           "error_compress_non_numeric",
			serverID:       "1",
			queryDisk:      "server",
			queryPath:      "data",
			queryCompress:  "abc",
			setupCtx:       authedCtx,
			setupRepo:      saveServerWithFilesAbility,
			setupArchiver:  func() *stubArchiver { return &stubArchiver{} },
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusBadRequest,
			wantError:      "compress must be",
		},
		{
			name:      "error_node_not_found_returns_404",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: func(t *testing.T, serverRepo *inmemory.ServerRepository, _ *inmemory.NodeRepository, rbacRepo *inmemory.RBACRepository) {
				t.Helper()
				now := time.Now()
				server := &domain.Server{
					ID: 1, UID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort: "short", Enabled: true, Installed: 1, Name: "S1",
					GameID: "g", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1",
					Dir: "servers/s1", CreatedAt: &now, UpdatedAt: &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser.ID, server.ID)

				ability := &domain.Ability{
					Name:       domain.AbilityNameGameServerFiles,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(server.ID),
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))
				require.NoError(t, rbacRepo.SavePermission(context.Background(), &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   new(testUser.ID),
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					Forbidden:  false,
				}))
			},
			setupArchiver:  func() *stubArchiver { return &stubArchiver{} },
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusNotFound,
			wantError:      "node not found",
		},
		{
			name:      "error_archiver_internal_error_returns_500_wrapped_build_manifest",
			serverID:  "1",
			queryDisk: "server",
			queryPath: "data",
			setupCtx:  authedCtx,
			setupRepo: saveServerWithFilesAbility,
			setupArchiver: func() *stubArchiver {
				return &stubArchiver{
					manifestErr: errBuildManifestXyz,
				}
			},
			setupGuard:     func() *fakeGuard { return &fakeGuard{} },
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			nodeRepo := inmemory.NewNodeRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()

			arch := tt.setupArchiver()
			guard := tt.setupGuard()
			handler := NewHandler(serverRepo, nodeRepo, rbacService, arch, guard, tt.limits, responder)

			tt.setupRepo(t, serverRepo, nodeRepo, rbacRepo)

			query := url.Values{}
			if tt.queryDisk != "" {
				query.Add("disk", tt.queryDisk)
			}
			if tt.queryPath != "" {
				query.Add("path", tt.queryPath)
			}
			if tt.queryCompress != "" {
				query.Add("compress", tt.queryCompress)
			}

			fullURL := "/api/file-manager/" + tt.serverID + "/download-archive"
			if len(query) > 0 {
				fullURL += "?" + query.Encode()
			}

			req := httptest.NewRequest(http.MethodGet, fullURL, nil)
			req = req.WithContext(tt.setupCtx())
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				assert.Contains(t, w.Body.String(), tt.wantError)
			}

			if tt.validate != nil {
				tt.validate(t, w, arch, guard)
			}
		})
	}
}

func TestArchiveFilename(t *testing.T) {
	tests := []struct {
		name     string
		rootName string
		path     string
		want     string
	}{
		{name: "uses_root_name", rootName: "data", path: "any", want: "data.zip"},
		{name: "falls_back_to_path_basename", rootName: "", path: "logs/sub", want: "sub.zip"},
		{name: "falls_back_to_archive", rootName: ".", path: "/", want: "archive.zip"},
		{name: "slash_only_root_falls_back", rootName: "/", path: "", want: "archive.zip"},
		{name: "dot_root_falls_back_to_path_basename", rootName: ".", path: "logs/x", want: "x.zip"},
		{name: "both_empty_returns_archive", rootName: "", path: "", want: "archive.zip"},
		{name: "dot_path_falls_back", rootName: "", path: ".", want: "archive.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := archiveFilename(tt.rootName, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContentDispositionHeader_RFC5987(t *testing.T) {
	header := contentDispositionHeader("кириллица.zip")
	assert.True(t, strings.HasPrefix(header, "attachment;"))
	assert.Contains(t, header, "filename*=UTF-8''")
}

func TestReadCompressLevel(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantValue int
		wantError string
	}{
		{name: "empty_returns_zero", raw: "", wantValue: 0, wantError: ""},
		{name: "zero", raw: "0", wantValue: 0, wantError: ""},
		{name: "nine", raw: "9", wantValue: 9, wantError: ""},
		{name: "whitespace_trimmed", raw: "  3  ", wantValue: 3, wantError: ""},
		{name: "negative", raw: "-1", wantValue: 0, wantError: "compress must be"},
		{name: "over_nine", raw: "10", wantValue: 0, wantError: "compress must be"},
		{name: "non_numeric", raw: "abc", wantValue: 0, wantError: "compress must be"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			urlStr := "/?"
			if tt.raw != "" {
				urlStr = "/?compress=" + url.QueryEscape(tt.raw)
			}
			req := httptest.NewRequest(http.MethodGet, urlStr, nil)

			// ACT
			got, err := readCompressLevel(req)

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.wantValue, got)

				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			assert.Equal(t, tt.wantValue, got)
		})
	}
}

func TestStripNonASCII(t *testing.T) {
	// stripNonASCII iterates by rune: every non-ASCII rune becomes one underscore.
	// Empty result falls back to "archive.zip".
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "pure_ascii_passthrough", in: "data.zip", want: "data.zip"},
		{name: "mixed_strips_non_ascii", in: "data_файл.zip", want: "data_____.zip"},
		{name: "quote_replaced", in: `a"b`, want: "a_b"},
		{name: "backslash_replaced", in: `a\b`, want: "a_b"},
		{name: "non_ascii_only_replaces_each_rune_with_underscore", in: "файл", want: "____"},
		{name: "empty_string_falls_back_to_archive", in: "", want: "archive.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripNonASCII(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMapManifestError(t *testing.T) {
	tests := []struct {
		name          string
		inErr         error
		wantStatus    int
		wantSubstring string
	}{
		{
			name:       "too_large_returns_413",
			inErr:      archiver.ErrTooLarge,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "too_many_files_returns_413",
			inErr:      archiver.ErrTooManyFiles,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "empty_manifest_returns_404",
			inErr:      archiver.ErrEmptyManifest,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "not_a_directory_returns_400",
			inErr:      archiver.ErrNotADirectory,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:          "unknown_error_wrapped_build_manifest",
			inErr:         errBuildManifestXyz,
			wantSubstring: "build manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got := mapManifestError(tt.inErr)
			require.Error(t, got)

			// ASSERT
			if tt.wantStatus != 0 {
				var statusErr interface{ HTTPStatus() int }
				require.ErrorAs(t, got, &statusErr, "result must expose HTTPStatus")
				assert.Equal(t, tt.wantStatus, statusErr.HTTPStatus())

				return
			}
			assert.Contains(t, got.Error(), tt.wantSubstring, "unknown errors must surface wrap layer")
			assert.Contains(t, got.Error(), tt.inErr.Error(), "underlying message must propagate")
		})
	}
}

type errNodeRepo struct {
	findErr error
}

func (e *errNodeRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	return nil, errNodeRepoFindAll
}

func (e *errNodeRepo) Find(
	_ context.Context, _ *filters.FindNode, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	return nil, e.findErr
}

func (e *errNodeRepo) Save(_ context.Context, _ *domain.Node) error {
	return errNodeRepoSave
}

func (e *errNodeRepo) UpdateGDaemonAPIToken(_ context.Context, _ uint, _ string, _ time.Time) error {
	return errNodeRepoSave
}

func (e *errNodeRepo) Delete(_ context.Context, _ uint) error {
	return errNodeRepoDelete
}

func TestHandler_ServeHTTP_NodeRepoError(t *testing.T) {
	// ARRANGE
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	saveServerWithFilesAbility(t, serverRepo, inmemory.NewNodeRepository(), rbacRepo)

	nodeRepo := &errNodeRepo{findErr: errNodeRepoDBDown}
	arch := &stubArchiver{}
	guard := &fakeGuard{}
	handler := NewHandler(serverRepo, nodeRepo, rbacService, arch, guard, Limits{}, responder)

	req := httptest.NewRequest(http.MethodGet, "/api/file-manager/1/download-archive?disk=server&path=data", nil)
	req = req.WithContext(authedCtx())
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, w.Code, "node-repo errors must surface as 500")
}
