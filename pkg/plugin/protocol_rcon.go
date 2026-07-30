package plugin

import (
	"context"

	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/pkg/errors"
)

// ErrRconAlreadyOpen is returned when Open is called on a client that already
// holds a connection. Overwriting the handle would strand the previous one in
// the registry until its deadline expires.
var ErrRconAlreadyOpen = errors.New("plugin rcon: connection is already open")

// pluginRconClient implements rcon.Client for a plugin-implemented protocol.
// The host owns the socket (dial + registry lifecycle); the plugin drives the
// wire protocol (auth handshake, framing, parsing) over the handle.
type pluginRconClient struct {
	runner     *ProtocolRunner
	pluginID   string
	protocolID string
	cfg        rcon.Config
	handle     uint64
	release    func()
}

func (c *pluginRconClient) Open(ctx context.Context) error {
	if c.handle != 0 {
		return ErrRconAlreadyOpen
	}

	handle, release, err := c.runner.openHandle(ctx, "tcp", c.cfg.Address, c.pluginID)
	if err != nil {
		return err
	}

	plugin, ok := c.runner.manager.GetPlugin(c.pluginID)
	if !ok || plugin.Protocol == nil {
		release()

		return errors.Wrapf(ErrPluginNotFound, "plugin: %s", c.pluginID)
	}

	callCtx, cancel := c.runner.callContext(ctx)
	defer cancel()

	resp, err := plugin.Protocol.RconOpen(callCtx, &protocol.RconOpenRequest{
		ProtocolId: c.protocolID,
		ConnHandle: handle,
		Password:   c.cfg.Password,
		Address:    c.cfg.Address,
	})
	if err != nil {
		release()

		return err
	}

	if resp.AuthFailed {
		release()

		return rcon.ErrAuthenticationFailed
	}

	if !resp.Ok {
		release()

		return errors.New(errStringOr(resp.Error, "plugin rcon open failed"))
	}

	c.handle = handle
	c.release = release

	return nil
}

func (c *pluginRconClient) Execute(ctx context.Context, command string) (string, error) {
	plugin, ok := c.runner.manager.GetPlugin(c.pluginID)
	if !ok || plugin.Protocol == nil {
		return "", errors.Wrapf(ErrPluginNotFound, "plugin: %s", c.pluginID)
	}

	callCtx, cancel := c.runner.callContext(ctx)
	defer cancel()

	resp, err := plugin.Protocol.RconExecute(callCtx, &protocol.RconExecuteRequest{
		ProtocolId: c.protocolID,
		ConnHandle: c.handle,
		Command:    command,
	})
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		return "", errors.New(*resp.Error)
	}

	return resp.Output, nil
}

func (c *pluginRconClient) Close() error {
	if c.handle == 0 {
		return nil
	}

	if plugin, ok := c.runner.manager.GetPlugin(c.pluginID); ok && plugin.Protocol != nil {
		callCtx, cancel := c.runner.callContext(context.Background())
		_, _ = plugin.Protocol.RconClose(callCtx, &protocol.RconCloseRequest{
			ProtocolId: c.protocolID,
			ConnHandle: c.handle,
		})
		cancel()
	}

	if c.release != nil {
		c.release()
	}

	c.handle = 0
	c.release = nil

	return nil
}

func errStringOr(s *string, fallback string) string {
	if s != nil && *s != "" {
		return *s
	}

	return fallback
}

// compile-time check that the adapter satisfies rcon.Client.
var _ rcon.Client = (*pluginRconClient)(nil)
