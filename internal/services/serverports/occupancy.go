package serverports

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
)

const noFreePortMessage = "no free port left in the port_range of the node"

// address is a server IP the way a socket binds it: textual variants of one
// IP compare equal, and the unspecified address overlaps every other.
type address struct {
	key      string
	wildcard bool
}

func parseAddress(s string) address {
	if ip, err := netip.ParseAddr(s); err == nil {
		ip = ip.Unmap()

		return address{key: ip.String(), wildcard: ip.IsUnspecified()}
	}

	return address{key: strings.ToLower(s)}
}

func (a address) overlaps(b address) bool {
	return a.wildcard || b.wildcard || a.key == b.key
}

type holder struct {
	address address
	server  *domain.Server
}

// occupancy is what the other servers of a node hold, indexed by port.
type occupancy struct {
	byPort map[int][]holder
	// previous is the stored record of the server being saved, when that
	// server already belongs to the node.
	previous *domain.Server
}

func newOccupancy(servers []domain.Server, serverID uint) occupancy {
	taken := occupancy{byPort: make(map[int][]holder)}

	for i := range servers {
		server := &servers[i]

		if serverID != 0 && server.ID == serverID {
			taken.previous = server

			continue
		}

		holding := holder{address: parseAddress(server.ServerIP), server: server}
		for _, port := range server.Ports() {
			taken.byPort[port] = append(taken.byPort[port], holding)
		}
	}

	return taken
}

// allocate tries the candidate addresses in order and settles on the first
// where the picked and the given ports are all free. It reports the error of
// the first address: the one a caller naming no address would expect.
func (o occupancy) allocate(node *domain.Node, server *domain.Server, pick Pick) error {
	addresses := []string{server.ServerIP}
	if pick.IP {
		if len(node.IPs) == 0 {
			return fieldError("server_ip", "the node has no IP address to pick from")
		}

		addresses = node.IPs
	}

	var pool domain.PortRange
	if pick.ServerPort || pick.QueryPort || pick.RconPort {
		var err error

		pool, err = node.PortRange()
		if err != nil {
			return api.NewValidationError("port_range of the node is invalid: " + err.Error())
		}
	}

	if pick.ServerPort && len(pool) == 0 {
		return fieldError("server_port", "the node has no port_range to pick a port from")
	}

	var firstErr error

	for _, ip := range addresses {
		candidate := *server
		candidate.ServerIP = ip

		err := o.fill(&candidate, pick, pool)
		if err == nil {
			err = o.verify(&candidate)
		}

		if err == nil {
			*server = candidate

			return nil
		}

		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// fill takes the lowest free ports of the pool for what pick asks. A query or
// RCON port stays unset without a pool, which means the server port.
func (o occupancy) fill(server *domain.Server, pick Pick, pool domain.PortRange) error {
	targets := []struct {
		wanted bool
		field  string
		set    func(port int)
	}{
		{pick.ServerPort, "server_port", func(port int) { server.ServerPort = port }},
		{pick.QueryPort && len(pool) > 0, "query_port", func(port int) { server.QueryPort = new(port) }},
		{pick.RconPort && len(pool) > 0, "rcon_port", func(port int) { server.RconPort = new(port) }},
	}

	for _, target := range targets {
		if !target.wanted {
			continue
		}

		port, ok := o.freePort(server, pool)
		if !ok {
			return fieldError(target.field, noFreePortMessage)
		}

		target.set(port)
	}

	return nil
}

// freePort returns the lowest port of the pool that no other server holds on
// the server's address and that the server does not use itself yet.
func (o occupancy) freePort(server *domain.Server, pool domain.PortRange) (int, bool) {
	addr := parseAddress(server.ServerIP)
	own := server.Ports()

	for port := range pool.All() {
		if !slices.Contains(own, port) && o.holderOf(addr, port) == nil {
			return port, true
		}
	}

	return 0, false
}

// verify reports the first port the server newly takes that another server
// already holds on an overlapping address.
func (o occupancy) verify(server *domain.Server) error {
	addr := parseAddress(server.ServerIP)

	for _, port := range o.newPorts(server) {
		if other := o.holderOf(addr, port); other != nil {
			return fieldError(portField(server, port), fmt.Sprintf(
				"port %d on %s is already used by server #%d (%s)", port, server.ServerIP, other.ID, other.Name,
			))
		}
	}

	return nil
}

// newPorts leaves out the ports the stored record of the server already holds
// on the same address.
func (o occupancy) newPorts(server *domain.Server) []int {
	ports := server.Ports()

	if o.previous == nil || parseAddress(o.previous.ServerIP) != parseAddress(server.ServerIP) {
		return ports
	}

	held := o.previous.Ports()

	return slices.DeleteFunc(ports, func(port int) bool {
		return slices.Contains(held, port)
	})
}

func (o occupancy) holderOf(addr address, port int) *domain.Server {
	for _, h := range o.byPort[port] {
		if h.address.overlaps(addr) {
			return h.server
		}
	}

	return nil
}

// portField names the request field carrying port, the server port first
// since the query and RCON ports often repeat it.
func portField(server *domain.Server, port int) string {
	switch {
	case port == server.ServerPort:
		return "server_port"
	case server.QueryPort != nil && *server.QueryPort == port:
		return "query_port"
	default:
		return "rcon_port"
	}
}

func fieldError(field, message string) error {
	return api.NewFieldValidationError(map[string][]string{field: {message}})
}
