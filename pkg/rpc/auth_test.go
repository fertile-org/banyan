package rpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestPeerIPFromContext(t *testing.T) {
	t.Run("valid peer info returns IP", func(t *testing.T) {
		ctx := peer.NewContext(context.Background(), &peer.Peer{
			Addr: &net.TCPAddr{IP: net.ParseIP("192.168.1.42"), Port: 12345},
		})
		ip, err := PeerIPFromContext(ctx)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ip != "192.168.1.42" {
			t.Errorf("expected '192.168.1.42', got %q", ip)
		}
	})

	t.Run("no peer info returns error", func(t *testing.T) {
		_, err := PeerIPFromContext(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got := status.Code(err); got != codes.Internal {
			t.Errorf("expected code Internal, got %v", got)
		}
	})

	t.Run("invalid peer address returns error", func(t *testing.T) {
		ctx := peer.NewContext(context.Background(), &peer.Peer{
			Addr: fakeAddr("not-a-host-port"),
		})
		_, err := PeerIPFromContext(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got := status.Code(err); got != codes.Internal {
			t.Errorf("expected code Internal, got %v", got)
		}
	})
}

// fakeAddr implements net.Addr with a custom string representation
// that will cause net.SplitHostPort to fail.
type fakeAddr string

func (f fakeAddr) Network() string { return "tcp" }
func (f fakeAddr) String() string  { return string(f) }
