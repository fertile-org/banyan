package rpc

import (
	"context"
	"net"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// PeerIPFromContext extracts the remote IP address from the gRPC peer info.
func PeerIPFromContext(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", status.Error(codes.Internal, "no peer info in context")
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return "", status.Errorf(codes.Internal, "invalid peer address: %v", err)
	}
	return host, nil
}
