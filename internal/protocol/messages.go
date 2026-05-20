package protocol

import "github.com/libp2p/go-libp2p/core/protocol"

const ProtocolID = protocol.ID("/claude-plans/1.0.0")

const maxMessageSize = 10 * 1024 * 1024 // 10MB

type Request struct {
	Type   string `json:"type"`
	PlanID string `json:"plan_id,omitempty"`
}

type ListResponse struct {
	PeerName string        `json:"peer_name"`
	PeerID   string        `json:"peer_id"`
	Plans    []PlanSummary `json:"plans"`
	Error    string        `json:"error,omitempty"`
}

type PlanSummary struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Modified string `json:"modified"`
}

type GetResponse struct {
	PeerName string `json:"peer_name"`
	PeerID   string `json:"peer_id"`
	PlanID   string `json:"plan_id"`
	Summary  string `json:"summary"`
	Content  string `json:"content"`
	Error    string `json:"error,omitempty"`
}
