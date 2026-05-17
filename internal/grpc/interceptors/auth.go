package interceptors

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	NodeIDKey         contextKey = "node_id"
	NodeKey           contextKey = "node"
	APIKeyMetadataKey            = "x-api-key"
	NodeIDMetadataKey            = "x-node-id"
	enrollFullMethod             = "/gameap.DaemonGateway/Enroll"
)

type AuthInterceptor struct {
	nodeRepo    repositories.NodeRepository
	requireMTLS bool
	logger      *slog.Logger
}

func NewAuthInterceptor(
	nodeRepo repositories.NodeRepository,
	requireMTLS bool,
	logger *slog.Logger,
) *AuthInterceptor {
	if logger == nil {
		logger = slog.Default()
	}

	return &AuthInterceptor{
		nodeRepo:    nodeRepo,
		requireMTLS: requireMTLS,
		logger:      logger,
	}
}

func (ai *AuthInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		if ai.requireMTLS {
			if err := ai.verifyMTLS(ctx); err != nil {
				return err
			}
		}

		return handler(srv, ss)
	}
}

func (ai *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if info.FullMethod == enrollFullMethod {
			return handler(ctx, req)
		}

		if ai.requireMTLS {
			if err := ai.verifyMTLS(ctx); err != nil {
				return nil, err
			}
		}

		nodeID, err := ai.extractAndVerifyAPIKey(ctx)
		if err != nil {
			return nil, err
		}

		if nodeID > 0 {
			ctx = context.WithValue(ctx, NodeIDKey, nodeID)
		}

		return handler(ctx, req)
	}
}

func (ai *AuthInterceptor) verifyMTLS(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if !ok {
		ai.logger.Warn("mTLS verification failed: no peer info")

		return status.Error(codes.Unauthenticated, "no peer information")
	}

	peerAddr := p.Addr.String()

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		ai.logger.Warn("mTLS verification failed: no TLS info",
			"peer", peerAddr,
		)

		return status.Error(codes.Unauthenticated, "no TLS info")
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		ai.logger.Warn("mTLS verification failed: no client certificate",
			"peer", peerAddr,
		)

		return status.Error(codes.Unauthenticated, "no client certificate")
	}

	cert := tlsInfo.State.PeerCertificates[0]
	ai.logger.Debug("mTLS verification succeeded",
		"peer", peerAddr,
		"client_cn", cert.Subject.CommonName,
		"client_not_after", cert.NotAfter.Format(time.RFC3339),
	)

	return nil
}

func (ai *AuthInterceptor) extractAndVerifyAPIKey(ctx context.Context) (uint64, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, nil
	}

	apiKeys := md.Get(APIKeyMetadataKey)
	if len(apiKeys) == 0 {
		return 0, nil
	}

	nodeIDs := md.Get(NodeIDMetadataKey)
	if len(nodeIDs) == 0 {
		return 0, status.Error(codes.InvalidArgument, "node ID required with API key")
	}

	nodeID, err := strconv.ParseUint(nodeIDs[0], 10, 64)
	if err != nil {
		return 0, status.Error(codes.InvalidArgument, "invalid node ID format")
	}

	nodes, err := ai.nodeRepo.Find(ctx, &filters.FindNode{IDs: []uint{uint(nodeID)}}, nil, nil)
	if err != nil {
		ai.logger.Error("failed to find node", "node_id", nodeID, "error", err)

		return 0, status.Error(codes.Internal, "failed to verify node")
	}

	if len(nodes) == 0 {
		return 0, status.Error(codes.NotFound, "node not found")
	}

	node := nodes[0]

	if !node.Enabled {
		return 0, status.Error(codes.PermissionDenied, "node is disabled")
	}

	// gdaemon_api_key is stored as a SHA-256 hash; hash the presented plaintext
	// before the constant-time comparison so a DB leak yields no usable key.
	if !secureCompare(pkgstrings.SHA256(apiKeys[0]), node.GdaemonAPIKey) {
		return 0, status.Error(codes.Unauthenticated, "invalid API key")
	}

	return nodeID, nil
}

func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func GetNodeIDFromContext(ctx context.Context) (uint64, bool) {
	v := ctx.Value(NodeIDKey)
	if v == nil {
		return 0, false
	}
	nodeID, ok := v.(uint64)

	return nodeID, ok
}

func GetNodeFromContext(ctx context.Context) (*domain.Node, bool) {
	v := ctx.Value(NodeKey)
	if v == nil {
		return nil, false
	}
	node, ok := v.(*domain.Node)

	return node, ok
}
