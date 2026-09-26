package serverports

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fieldErrors interface {
	FieldErrors() map[string][]string
}

type httpStatus interface {
	HTTPStatus() int
}

func assertFieldError(t *testing.T, err error, field, wantError string) {
	t.Helper()

	require.Error(t, err)
	assert.Contains(t, err.Error(), wantError)

	var withFields fieldErrors
	require.ErrorAs(t, err, &withFields)
	require.Len(t, withFields.FieldErrors()[field], 1, "errors of %s: %v", field, withFields.FieldErrors())
	assert.Contains(t, withFields.FieldErrors()[field][0], wantError)
}

// setupRepo saves the given servers with repository-assigned IDs, so a server
// created by the test gets the next free ID instead of overwriting one.
func setupRepo(t *testing.T, servers []domain.Server) (*inmemory.ServerRepository, []domain.Server) {
	t.Helper()

	repo := inmemory.NewServerRepository()
	saved := make([]domain.Server, 0, len(servers))

	for _, server := range servers {
		require.NoError(t, repo.Save(context.Background(), &server))
		saved = append(saved, server)
	}

	return repo, saved
}

func nodeWith(ips []string, portRange string) *domain.Node {
	node := &domain.Node{ID: 1, IPs: ips}
	if portRange != "" {
		node.Metadata = domain.Metadata{"port_range": portRange}
	}

	return node
}

func TestService_Allocate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		node     *domain.Node
		existing []domain.Server
		server   domain.Server
		pick     Pick

		wantIP         string
		wantServerPort int
		wantQueryPort  *int
		wantRconPort   *int
		wantField      string
		wantError      string
	}{
		{
			name:           "picks_lowest_free_port_of_range",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			existing:       []domain.Server{{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}},
			server:         domain.Server{DSID: 1},
			pick:           Pick{IP: true, ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27016,
		},
		{
			name: "skips_query_and_rcon_ports_of_other_servers",
			node: nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			existing: []domain.Server{{
				DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, QueryPort: new(27016), RconPort: new(27017),
			}},
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:           Pick{ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27018,
		},
		{
			name:           "other_address_does_not_block",
			node:           nodeWith([]string{"10.0.0.5", "10.0.0.6"}, "27015-27020"),
			existing:       []domain.Server{{DSID: 1, ServerIP: "10.0.0.6", ServerPort: 27015}},
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:           Pick{ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
		},
		{
			name:           "other_node_does_not_block",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			existing:       []domain.Server{{DSID: 2, ServerIP: "10.0.0.5", ServerPort: 27015}},
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:           Pick{ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
		},
		{
			name: "deleted_server_does_not_block",
			node: nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			existing: []domain.Server{{
				DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, DeletedAt: new(time.Now()),
			}},
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:           Pick{ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
		},
		{
			name:           "disabled_server_still_blocks",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			existing:       []domain.Server{{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Enabled: false}},
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:           Pick{ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27016,
		},
		{
			name:           "wildcard_holder_blocks_every_address",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			existing:       []domain.Server{{DSID: 1, ServerIP: "0.0.0.0", ServerPort: 27015}},
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:           Pick{ServerPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27016,
		},
		{
			name:           "wildcard_server_is_blocked_by_every_address",
			node:           nodeWith([]string{"::"}, "27015-27020"),
			existing:       []domain.Server{{DSID: 1, ServerIP: "10.0.0.6", ServerPort: 27015}},
			server:         domain.Server{DSID: 1, ServerIP: "::"},
			pick:           Pick{ServerPort: true},
			wantIP:         "::",
			wantServerPort: 27016,
		},
		{
			name:           "picks_query_and_rcon_after_server_port",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			server:         domain.Server{DSID: 1},
			pick:           Pick{IP: true, ServerPort: true, QueryPort: true, RconPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
			wantQueryPort:  new(27016),
			wantRconPort:   new(27017),
		},
		{
			name:           "picked_rcon_skips_own_query_port",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 30000, QueryPort: new(27015)},
			pick:           Pick{RconPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 30000,
			wantQueryPort:  new(27015),
			wantRconPort:   new(27016),
		},
		{
			name:           "query_and_rcon_stay_unset_without_range",
			node:           nodeWith([]string{"10.0.0.5"}, ""),
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015},
			pick:           Pick{QueryPort: true, RconPort: true},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
		},
		{
			name:           "explicit_port_outside_range_is_allowed",
			node:           nodeWith([]string{"10.0.0.5"}, "27015-27020"),
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 30000},
			wantIP:         "10.0.0.5",
			wantServerPort: 30000,
		},
		{
			name: "own_ports_may_repeat",
			node: nodeWith([]string{"10.0.0.5"}, ""),
			server: domain.Server{
				DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, QueryPort: new(27015), RconPort: new(27015),
			},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
			wantQueryPort:  new(27015),
			wantRconPort:   new(27015),
		},
		{
			name:           "picks_next_address_when_first_is_full",
			node:           nodeWith([]string{"10.0.0.5", "10.0.0.6"}, "27015"),
			existing:       []domain.Server{{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}},
			server:         domain.Server{DSID: 1},
			pick:           Pick{IP: true, ServerPort: true},
			wantIP:         "10.0.0.6",
			wantServerPort: 27015,
		},
		{
			name:           "picks_address_where_given_port_is_free",
			node:           nodeWith([]string{"10.0.0.5", "10.0.0.6"}, ""),
			existing:       []domain.Server{{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}},
			server:         domain.Server{DSID: 1, ServerPort: 27015},
			pick:           Pick{IP: true},
			wantIP:         "10.0.0.6",
			wantServerPort: 27015,
		},
		{
			name:           "invalid_stored_range_is_ignored_when_nothing_is_picked",
			node:           nodeWith([]string{"10.0.0.5"}, "abc"),
			server:         domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015},
			wantIP:         "10.0.0.5",
			wantServerPort: 27015,
		},
		{
			name: "given_port_taken",
			node: nodeWith([]string{"10.0.0.5"}, ""),
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Public CS"},
			},
			server:    domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015},
			wantField: "server_port",
			wantError: "port 27015 on 10.0.0.5 is already used by server #1 (Public CS)",
		},
		{
			name:     "given_rcon_port_taken",
			node:     nodeWith([]string{"10.0.0.5"}, ""),
			existing: []domain.Server{{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27016, Name: "Rust"}},
			server: domain.Server{
				DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, RconPort: new(27016),
			},
			wantField: "rcon_port",
			wantError: "port 27016 on 10.0.0.5 is already used by server #1 (Rust)",
		},
		{
			name:      "mapped_ipv6_is_the_same_address",
			node:      nodeWith([]string{"10.0.0.5"}, ""),
			existing:  []domain.Server{{DSID: 1, ServerIP: "::ffff:10.0.0.5", ServerPort: 27015, Name: "Mapped"}},
			server:    domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015},
			wantField: "server_port",
			wantError: "already used by server #1 (Mapped)",
		},
		{
			name:      "hostnames_compare_case_insensitively",
			node:      nodeWith([]string{"game.example.com"}, ""),
			existing:  []domain.Server{{DSID: 1, ServerIP: "Game.Example.com", ServerPort: 27015, Name: "Host"}},
			server:    domain.Server{DSID: 1, ServerIP: "game.example.com", ServerPort: 27015},
			wantField: "server_port",
			wantError: "already used by server #1 (Host)",
		},
		{
			name: "range_exhausted",
			node: nodeWith([]string{"10.0.0.5"}, "27015-27016"),
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015},
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27016},
			},
			server:    domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:      Pick{ServerPort: true},
			wantField: "server_port",
			wantError: "no free port left in the port_range of the node",
		},
		{
			name:      "no_port_left_for_query_port",
			node:      nodeWith([]string{"10.0.0.5"}, "27015"),
			server:    domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:      Pick{ServerPort: true, QueryPort: true},
			wantField: "query_port",
			wantError: "no free port left in the port_range of the node",
		},
		{
			name:      "no_range_to_pick_server_port_from",
			node:      nodeWith([]string{"10.0.0.5"}, ""),
			server:    domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:      Pick{ServerPort: true},
			wantField: "server_port",
			wantError: "the node has no port_range to pick a port from",
		},
		{
			name:      "no_address_to_pick",
			node:      nodeWith(nil, "27015-27020"),
			server:    domain.Server{DSID: 1},
			pick:      Pick{IP: true, ServerPort: true},
			wantField: "server_ip",
			wantError: "the node has no IP address to pick from",
		},
		{
			name:      "invalid_stored_range",
			node:      nodeWith([]string{"10.0.0.5"}, "abc"),
			server:    domain.Server{DSID: 1, ServerIP: "10.0.0.5"},
			pick:      Pick{ServerPort: true},
			wantError: `port_range of the node is invalid: invalid entry "abc"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, _ := setupRepo(t, tt.existing)
			service := NewService(repo, locker.NewInMemoryLocker())
			server := tt.server
			saves := 0

			err := service.Allocate(context.Background(), tt.node, &server, tt.pick, func(ctx context.Context) error {
				saves++

				return repo.Save(ctx, &server)
			})

			if tt.wantError != "" {
				if tt.wantField != "" {
					assertFieldError(t, err, tt.wantField, tt.wantError)
				} else {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantError)
				}

				var withStatus httpStatus
				require.ErrorAs(t, err, &withStatus)
				assert.Equal(t, http.StatusUnprocessableEntity, withStatus.HTTPStatus())
				assert.Zero(t, saves, "a rejected server must not be saved")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, 1, saves)
			assert.Equal(t, tt.wantIP, server.ServerIP)
			assert.Equal(t, tt.wantServerPort, server.ServerPort)
			assert.Equal(t, tt.wantQueryPort, server.QueryPort)
			assert.Equal(t, tt.wantRconPort, server.RconPort)
		})
	}
}

func TestService_Check(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing []domain.Server
		// update changes the stored record of existing[0] before the check.
		update    func(server *domain.Server)
		wantField string
		wantError string
	}{
		{
			name: "new_port_taken_by_other_server",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27016, Name: "Edited"},
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Holder"},
			},
			update:    func(server *domain.Server) { server.ServerPort = 27015 },
			wantField: "server_port",
			wantError: "port 27015 on 10.0.0.5 is already used by server #2 (Holder)",
		},
		{
			name: "new_query_port_taken_by_other_server",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Edited"},
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27016, Name: "Holder"},
			},
			update:    func(server *domain.Server) { server.QueryPort = new(27016) },
			wantField: "query_port",
			wantError: "port 27016 on 10.0.0.5 is already used by server #2 (Holder)",
		},
		{
			name: "clash_that_predates_the_check_does_not_block_edits",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Edited"},
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Twin"},
			},
			update: func(server *domain.Server) { server.Name = "Renamed" },
		},
		{
			name: "free_new_port_passes_despite_old_clash",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Edited"},
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Twin"},
			},
			update: func(server *domain.Server) { server.RconPort = new(27020) },
		},
		{
			name: "address_change_checks_every_port",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.6", ServerPort: 27015, Name: "Edited"},
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Holder"},
			},
			update:    func(server *domain.Server) { server.ServerIP = "10.0.0.5" },
			wantField: "server_port",
			wantError: "already used by server #2 (Holder)",
		},
		{
			name: "move_to_other_node_checks_every_port",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Edited"},
				{DSID: 2, ServerIP: "10.0.0.5", ServerPort: 27015, Name: "Holder"},
			},
			update:    func(server *domain.Server) { server.DSID = 2 },
			wantField: "server_port",
			wantError: "already used by server #2 (Holder)",
		},
		{
			name: "own_stored_record_does_not_conflict",
			existing: []domain.Server{
				{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015, RconPort: new(27016), Name: "Edited"},
			},
			update: func(server *domain.Server) { server.QueryPort = new(27015) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, saved := setupRepo(t, tt.existing)
			service := NewService(repo, locker.NewInMemoryLocker())
			server := saved[0]
			tt.update(&server)
			saves := 0

			err := service.Check(context.Background(), &server, func(ctx context.Context) error {
				saves++

				return repo.Save(ctx, &server)
			})

			if tt.wantError != "" {
				assertFieldError(t, err, tt.wantField, tt.wantError)
				assert.Zero(t, saves, "a rejected server must not be saved")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, 1, saves)
		})
	}
}

func TestService_SaveErrorIsReturned(t *testing.T) {
	t.Parallel()

	repo, _ := setupRepo(t, nil)
	service := NewService(repo, locker.NewInMemoryLocker())
	server := domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}
	saveErr := errors.New("db down")

	err := service.Check(context.Background(), &server, func(context.Context) error {
		return saveErr
	})

	require.ErrorIs(t, err, saveErr)
}

// The save sleeps so that, without the node lock, concurrent requests would
// all see the same free ports before any of them is written.
func TestService_Allocate_ConcurrentRequestsGetDistinctPorts(t *testing.T) {
	t.Parallel()

	const requests = 20

	repo, _ := setupRepo(t, nil)
	service := NewService(repo, locker.NewInMemoryLocker())
	node := nodeWith([]string{"10.0.0.5"}, "27015-27024")

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		ports   []int
		refused int
	)

	for range requests {
		wg.Go(func() {
			server := domain.Server{DSID: 1}

			err := service.Allocate(context.Background(), node, &server, Pick{IP: true, ServerPort: true},
				func(ctx context.Context) error {
					time.Sleep(time.Millisecond)

					return repo.Save(ctx, &server)
				},
			)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				assert.Contains(t, err.Error(), "no free port left in the port_range of the node")
				refused++

				return
			}

			ports = append(ports, server.ServerPort)
		})
	}

	wg.Wait()

	assert.ElementsMatch(t, []int{27015, 27016, 27017, 27018, 27019, 27020, 27021, 27022, 27023, 27024}, ports)
	assert.Equal(t, requests-len(ports), refused)
}

func TestService_LockHeldByAnotherRequest(t *testing.T) {
	t.Parallel()

	repo, _ := setupRepo(t, nil)
	locks := locker.NewInMemoryLocker()
	service := NewService(repo, locks)
	service.lockWait = 100 * time.Millisecond

	held, err := locks.Acquire(context.Background(), lockKeyPrefix+"1", time.Minute)
	require.NoError(t, err)
	defer func() { _ = held.Release(context.Background()) }()

	server := domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}
	saves := 0

	err = service.Check(context.Background(), &server, func(context.Context) error {
		saves++

		return nil
	})

	require.ErrorIs(t, err, ErrNodeBusy)

	var withStatus httpStatus
	require.ErrorAs(t, err, &withStatus)
	assert.Equal(t, http.StatusServiceUnavailable, withStatus.HTTPStatus())
	assert.Zero(t, saves)
}

func TestService_LockWaitEndsWithRequest(t *testing.T) {
	t.Parallel()

	repo, _ := setupRepo(t, nil)
	locks := locker.NewInMemoryLocker()
	service := NewService(repo, locks)

	held, err := locks.Acquire(context.Background(), lockKeyPrefix+"1", time.Minute)
	require.NoError(t, err)
	defer func() { _ = held.Release(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	server := domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}

	err = service.Check(ctx, &server, func(context.Context) error { return nil })

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, ErrNodeBusy)
}

func TestService_ReleasesLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		save func(context.Context) error
	}{
		{
			name: "after_save",
			save: func(context.Context) error { return nil },
		},
		{
			name: "after_failed_save",
			save: func(context.Context) error { return errors.New("db down") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo, _ := setupRepo(t, nil)
			locks := locker.NewInMemoryLocker()
			service := NewService(repo, locks)
			server := domain.Server{DSID: 1, ServerIP: "10.0.0.5", ServerPort: 27015}

			_ = service.Check(context.Background(), &server, tt.save)

			lock, err := locks.Acquire(context.Background(), lockKeyPrefix+"1", time.Second)
			require.NoError(t, err, "the node lock must be free once Check returns")
			require.NoError(t, lock.Release(context.Background()))
		})
	}
}
