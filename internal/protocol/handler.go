package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/p2p-claude-plans/p2p-claude-plans/internal/planstore"
)

func RegisterHandler(h host.Host, store *planstore.Store, peerName string) {
	h.SetStreamHandler(ProtocolID, func(s network.Stream) {
		defer s.Close()
		s.SetDeadline(time.Now().Add(30 * time.Second))

		req, err := readMessage[Request](s)
		if err != nil {
			log.Printf("protocol: read from %s: %v", s.Conn().RemotePeer().String()[:16], err)
			return
		}

		switch req.Type {
		case "list":
			handleList(s, store, peerName, h)
		case "get":
			handleGet(s, store, peerName, h, req.PlanID)
		default:
			writeMessage(s, &ListResponse{Error: "unknown request type"})
		}
	})
}

func handleList(s network.Stream, store *planstore.Store, peerName string, h host.Host) {
	plans := store.ListPlans()
	summaries := make([]PlanSummary, len(plans))
	for i, p := range plans {
		summaries[i] = PlanSummary{
			ID:       p.ID,
			Summary:  p.Summary,
			Modified: p.Modified.Format(time.RFC3339),
		}
	}
	writeMessage(s, &ListResponse{
		PeerName: peerName,
		PeerID:   h.ID().String(),
		Plans:    summaries,
	})
}

func handleGet(s network.Stream, store *planstore.Store, peerName string, h host.Host, planID string) {
	content, err := store.GetPlanContent(planID)
	if err != nil {
		writeMessage(s, &GetResponse{Error: fmt.Sprintf("plan not found: %s", planID)})
		return
	}
	plan, _ := store.GetPlan(planID)
	writeMessage(s, &GetResponse{
		PeerName: peerName,
		PeerID:   h.ID().String(),
		PlanID:   planID,
		Summary:  plan.Summary,
		Content:  content,
	})
}

func readMessage[T any](r io.Reader) (*T, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var msg T
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func writeMessage(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
