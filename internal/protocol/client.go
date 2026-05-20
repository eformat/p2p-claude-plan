package protocol

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

func QueryPeerList(ctx context.Context, h host.Host, peerID peer.ID, timeout time.Duration) (*ListResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s, err := h.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("open stream to %s: %w", peerID.String()[:16], err)
	}
	defer s.Close()

	s.SetDeadline(time.Now().Add(timeout))

	if err := writeMessage(s, &Request{Type: "list"}); err != nil {
		return nil, fmt.Errorf("write to %s: %w", peerID.String()[:16], err)
	}
	s.CloseWrite()

	resp, err := readMessage[ListResponse](s)
	if err != nil {
		return nil, fmt.Errorf("read from %s: %w", peerID.String()[:16], err)
	}
	return resp, nil
}

func QueryPeerGet(ctx context.Context, h host.Host, peerID peer.ID, planID string, timeout time.Duration) (*GetResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	s, err := h.NewStream(ctx, peerID, ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("open stream to %s: %w", peerID.String()[:16], err)
	}
	defer s.Close()

	s.SetDeadline(time.Now().Add(timeout))

	if err := writeMessage(s, &Request{Type: "get", PlanID: planID}); err != nil {
		return nil, fmt.Errorf("write to %s: %w", peerID.String()[:16], err)
	}
	s.CloseWrite()

	resp, err := readMessage[GetResponse](s)
	if err != nil {
		return nil, fmt.Errorf("read from %s: %w", peerID.String()[:16], err)
	}
	return resp, nil
}
