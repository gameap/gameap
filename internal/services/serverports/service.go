// Package serverports keeps the ports of game servers unique on a node and
// picks the ones a create request leaves out.
//
// A port is taken when another server of the same node (soft-deleted ones
// aside) uses it as its server, query or RCON port on the same address;
// 0.0.0.0 and :: overlap every address. The ports of one server may repeat:
// a Source server runs RCON over TCP on its UDP game port.
//
// The check and the save run under a per-node distributed lock, so two panel
// instances cannot hand out the same port between looking and writing.
package serverports

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

const (
	lockKeyPrefix = "server-ports:"
	// lockTTL only matters when an instance dies holding the lock: the
	// critical section is a query and an insert.
	lockTTL = 10 * time.Second
	// defaultLockWait outlasts lockTTL, so a lock left by a crashed instance
	// expires while a request is still waiting for it.
	defaultLockWait = 15 * time.Second
	lockRetryDelay  = 50 * time.Millisecond
	releaseTimeout  = 5 * time.Second
)

var ErrNodeBusy = errors.New("another request is assigning ports on the node, retry later")

// Pick lists what the panel chooses for a server instead of the caller.
type Pick struct {
	IP         bool
	ServerPort bool
	QueryPort  bool
	RconPort   bool
}

type Service struct {
	servers  repositories.ServerRepository
	locks    locker.Locker
	lockWait time.Duration
}

func NewService(servers repositories.ServerRepository, locks locker.Locker) *Service {
	return &Service{
		servers:  servers,
		locks:    locks,
		lockWait: defaultLockWait,
	}
}

// Allocate fills in what pick asks for from the node's IP addresses and its
// port_range, checks that every port of the server is free on the node and
// runs save, all under the node's lock. A query or RCON port is only picked
// when the node has a port_range; without one it stays unset, which means the
// server port.
func (s *Service) Allocate(
	ctx context.Context,
	node *domain.Node,
	server *domain.Server,
	pick Pick,
	save func(context.Context) error,
) error {
	return s.withNodeLock(ctx, node.ID, func(ctx context.Context) error {
		taken, err := s.occupancy(ctx, node.ID, server.ID)
		if err != nil {
			return err
		}

		if err := taken.allocate(node, server, pick); err != nil {
			return err
		}

		return save(ctx)
	})
}

// Check runs save under the lock of the server's node once the ports the
// server newly takes there are free. Ports its stored record already holds on
// the same address are not checked again, so a clash that predates the check
// does not block unrelated edits.
func (s *Service) Check(ctx context.Context, server *domain.Server, save func(context.Context) error) error {
	return s.withNodeLock(ctx, server.DSID, func(ctx context.Context) error {
		taken, err := s.occupancy(ctx, server.DSID, server.ID)
		if err != nil {
			return err
		}

		if err := taken.verify(server); err != nil {
			return err
		}

		return save(ctx)
	})
}

func (s *Service) occupancy(ctx context.Context, nodeID, serverID uint) (occupancy, error) {
	servers, err := s.servers.Find(ctx, filters.FindServerByNodeIDs(nodeID), nil, nil)
	if err != nil {
		return occupancy{}, errors.WithMessage(err, "failed to find servers of the node")
	}

	return newOccupancy(servers, serverID), nil
}

func (s *Service) withNodeLock(ctx context.Context, nodeID uint, fn func(context.Context) error) error {
	lock, err := s.acquire(ctx, nodeID)
	if err != nil {
		return err
	}

	defer s.release(ctx, lock)

	return fn(ctx)
}

// acquire polls the locker, which only offers a try-lock; the lock is held
// for milliseconds, so a short retry delay keeps the wait close to that.
func (s *Service) acquire(ctx context.Context, nodeID uint) (locker.Lock, error) {
	key := lockKeyPrefix + strconv.FormatUint(uint64(nodeID), 10)

	waitCtx, cancel := context.WithTimeout(ctx, s.lockWait)
	defer cancel()

	for {
		lock, err := s.locks.Acquire(waitCtx, key, lockTTL)
		if err == nil {
			return lock, nil
		}

		if !errors.Is(err, locker.ErrLocked) && waitCtx.Err() == nil {
			return nil, errors.WithMessage(err, "failed to lock ports of the node")
		}

		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return nil, errors.Wrap(ctx.Err(), "failed to lock ports of the node")
			}

			return nil, api.WrapHTTPError(ErrNodeBusy, http.StatusServiceUnavailable)
		case <-time.After(lockRetryDelay):
		}
	}
}

// release outlives a cancelled request: the server row is already written,
// and a lock left behind would stall the node's next create until its TTL.
func (s *Service) release(ctx context.Context, lock locker.Lock) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
	defer cancel()

	if err := lock.Release(ctx); err != nil {
		slog.WarnContext(ctx, "failed to release ports lock of the node", slog.String("error", err.Error()))
	}
}
