//go:build wasip1

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
)

//go:embed frontend/dist/plugin.js
var frontendBundle []byte

func (p *ServerLoggerPlugin) GetFrontendBundle(
	_ context.Context,
	_ *pluginproto.GetFrontendBundleRequest,
) (*pluginproto.GetFrontendBundleResponse, error) {
	return &pluginproto.GetFrontendBundleResponse{
		HasBundle: true,
		Bundle:    frontendBundle,
	}, nil
}

func (p *ServerLoggerPlugin) GetHTTPRoutes(
	_ context.Context,
	_ *pluginproto.GetHTTPRoutesRequest,
) (*pluginproto.GetHTTPRoutesResponse, error) {
	return &pluginproto.GetHTTPRoutesResponse{
		Routes: []*pluginproto.HTTPRoute{
			{
				Path:         "/status",
				Methods:      []string{"GET"},
				RequiresAuth: false,
				AdminOnly:    false,
				Description:  "Get plugin status",
			},
			{
				Path:         "/stats",
				Methods:      []string{"GET"},
				RequiresAuth: true,
				AdminOnly:    false,
				Description:  "Get plugin statistics",
			},
			{
				Path:         "/servers/{id}",
				Methods:      []string{"GET"},
				RequiresAuth: true,
				AdminOnly:    false,
				Description:  "Get server info by ID",
			},
		},
	}, nil
}

func (p *ServerLoggerPlugin) HandleHTTPRequest(
	ctx context.Context,
	req *pluginproto.HTTPRequest,
) (*pluginproto.HTTPResponse, error) {
	switch req.Path {
	case "/status":
		return p.handleStatus()
	case "/stats":
		return p.handleStats(req)
	default:
		if serverID, ok := req.PathParams["id"]; ok {
			return p.handleGetServer(ctx, serverID, req)
		}
		return errorResponse(http.StatusNotFound, "not found"), nil
	}
}

func (p *ServerLoggerPlugin) handleStatus() (*pluginproto.HTTPResponse, error) {
	body := `{"status":"ok","plugin":"server-logger","version":"1.0.0"}`
	return jsonResponse(http.StatusOK, body), nil
}

func (p *ServerLoggerPlugin) handleStats(req *pluginproto.HTTPRequest) (*pluginproto.HTTPResponse, error) {
	var userLogin string
	if req.Session != nil && req.Session.User != nil {
		userLogin = req.Session.User.Login
	}

	body := fmt.Sprintf(`{"events_processed":%d,"requested_by":"%s"}`,
		eventCounter.Load(),
		escapeJSON(userLogin),
	)
	return jsonResponse(http.StatusOK, body), nil
}

func (p *ServerLoggerPlugin) handleGetServer(
	ctx context.Context,
	serverID string,
	req *pluginproto.HTTPRequest,
) (*pluginproto.HTTPResponse, error) {
	id, err := strconv.ParseUint(serverID, 10, 64)
	if err != nil {
		return errorResponse(http.StatusBadRequest, "invalid server ID"), nil
	}

	resp, err := serversRepo.GetServer(ctx, &servers.GetServerRequest{Id: id})
	if err != nil {
		logger.Error("Failed to get server", slog.String("error", err.Error()))
		return errorResponse(http.StatusInternalServerError, "internal error"), nil
	}

	if !resp.Found || resp.Server == nil {
		return errorResponse(http.StatusNotFound, "server not found"), nil
	}

	var userLogin string
	if req.Session != nil && req.Session.User != nil {
		userLogin = req.Session.User.Login
	}

	body := fmt.Sprintf(
		`{"server":{"id":%d,"name":"%s","ip":"%s","port":%d},"requested_by":"%s"}`,
		resp.Server.Id,
		escapeJSON(resp.Server.Name),
		escapeJSON(resp.Server.ServerIp),
		resp.Server.ServerPort,
		escapeJSON(userLogin),
	)
	return jsonResponse(http.StatusOK, body), nil
}

func jsonResponse(statusCode int, body string) *pluginproto.HTTPResponse {
	return &pluginproto.HTTPResponse{
		StatusCode: int32(statusCode),
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte(body),
	}
}

func errorResponse(statusCode int, message string) *pluginproto.HTTPResponse {
	body := fmt.Sprintf(`{"error":"%s"}`, escapeJSON(message))
	return jsonResponse(statusCode, body)
}

func escapeJSON(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			result = append(result, '\\', '"')
		case '\\':
			result = append(result, '\\', '\\')
		case '\n':
			result = append(result, '\\', 'n')
		case '\r':
			result = append(result, '\\', 'r')
		case '\t':
			result = append(result, '\\', 't')
		default:
			if c < 0x20 {
				result = append(result, fmt.Sprintf("\\u%04x", c)...)
			} else {
				result = append(result, c)
			}
		}
	}
	return string(result)
}
