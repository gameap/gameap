package plugin

import (
	"github.com/gameap/gameap/internal/domain"
	domainproto "github.com/gameap/gameap/pkg/proto"
)

// domainServerToProto converts a domain.Server to proto.Server.
func domainServerToProto(s *domain.Server) *domainproto.Server {
	if s == nil {
		return nil
	}

	var queryPort, rconPort *int32
	if s.QueryPort != nil {
		qp := int32(*s.QueryPort) //nolint:gosec
		queryPort = &qp
	}
	if s.RconPort != nil {
		rp := int32(*s.RconPort) //nolint:gosec
		rconPort = &rp
	}

	return &domainproto.Server{
		Id:            uint64(s.ID),
		Uuid:          s.UUID.String(),
		UuidShort:     s.UUIDShort,
		Enabled:       s.Enabled,
		Installed:     domainproto.ServerInstalledStatus(s.Installed), //nolint:gosec
		Blocked:       s.Blocked,
		Name:          s.Name,
		GameId:        s.GameID,
		DsId:          uint64(s.DSID),
		GameModId:     uint64(s.GameModID),
		ServerIp:      s.ServerIP,
		ServerPort:    int32(s.ServerPort), //nolint:gosec
		QueryPort:     queryPort,
		RconPort:      rconPort,
		Dir:           s.Dir,
		SuUser:        s.SuUser,
		StartCommand:  s.StartCommand,
		ProcessActive: s.ProcessActive,
	}
}

// DomainUserToProto converts a user for plugins: identity and timestamps
// only, the same fields the HTTP session carries — never credentials or
// second-factor state.
func DomainUserToProto(u *domain.User) *domainproto.User {
	if u == nil {
		return nil
	}

	user := &domainproto.User{
		Id:    uint64(u.ID),
		Login: u.Login,
		Email: u.Email,
		Name:  u.Name,
	}

	if u.CreatedAt != nil {
		user.CreatedAt = new(u.CreatedAt.Unix())
	}

	if u.UpdatedAt != nil {
		user.UpdatedAt = new(u.UpdatedAt.Unix())
	}

	return user
}

// DomainNodeToProto converts a node for plugins, the field set of the
// gameap-nodes host library: no daemon credentials or certificates.
func DomainNodeToProto(n *domain.Node) *domainproto.Node {
	if n == nil {
		return nil
	}

	node := &domainproto.Node{
		Id:          uint64(n.ID),
		Name:        n.Name,
		Enabled:     n.Enabled,
		Os:          string(n.OS),
		Location:    n.Location,
		Provider:    n.Provider,
		Ips:         n.IPs,
		WorkPath:    n.WorkPath,
		GdaemonHost: n.GdaemonHost,
		GdaemonPort: int32(n.GdaemonPort), //nolint:gosec
	}

	if n.CreatedAt != nil {
		node.CreatedAt = new(n.CreatedAt.Unix())
	}

	if n.UpdatedAt != nil {
		node.UpdatedAt = new(n.UpdatedAt.Unix())
	}

	return node
}

// DomainServerSettingToProto converts a server setting for plugins.
func DomainServerSettingToProto(s domain.ServerSetting) *domainproto.ServerSetting {
	value, _ := s.Value.String()

	return &domainproto.ServerSetting{
		Id:       uint64(s.ID),
		ServerId: uint64(s.ServerID),
		Name:     s.Name,
		Value:    value,
	}
}
