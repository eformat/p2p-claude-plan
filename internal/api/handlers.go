package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/p2p-claude-plans/p2p-claude-plans/internal/protocol"
)

type planSummaryJSON struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Modified string `json:"modified"`
}

type peerPlansJSON struct {
	PeerName string            `json:"peer_name"`
	PeerID   string            `json:"peer_id"`
	IsLocal  bool              `json:"is_local"`
	Plans    []planSummaryJSON `json:"plans"`
	Error    string            `json:"error,omitempty"`
}

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	localPlans := s.store.ListPlans()
	summaries := make([]planSummaryJSON, len(localPlans))
	for i, p := range localPlans {
		summaries[i] = planSummaryJSON{
			ID:       p.ID,
			Summary:  p.Summary,
			Modified: p.Modified.Format(time.RFC3339),
		}
	}

	localPeerID := "local"
	if s.node != nil {
		localPeerID = s.node.PeerID().String()
	}

	result := []peerPlansJSON{{
		PeerName: s.peerName,
		PeerID:   localPeerID,
		IsLocal:  true,
		Plans:    summaries,
	}}

	if s.node != nil {
		peers := s.node.Peers()
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, pid := range peers {
			if pid == s.node.PeerID() {
				continue
			}
			wg.Add(1)
			go func(pid peer.ID) {
				defer wg.Done()
				resp, err := protocol.QueryPeerList(ctx, s.node.Host, pid, s.timeout)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					result = append(result, peerPlansJSON{
						PeerID: pid.String(),
						Error:  err.Error(),
					})
					return
				}
				plans := make([]planSummaryJSON, len(resp.Plans))
				for i, p := range resp.Plans {
					plans[i] = planSummaryJSON{
						ID:       p.ID,
						Summary:  p.Summary,
						Modified: p.Modified,
					}
				}
				result = append(result, peerPlansJSON{
					PeerName: resp.PeerName,
					PeerID:   resp.PeerID,
					Plans:    plans,
				})
			}(pid)
		}
		wg.Wait()
	}

	writeJSON(w, result)
}

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	peerID := r.PathValue("peerID")
	planID := r.PathValue("planID")

	if !isValidPlanID(planID) {
		http.Error(w, "invalid plan_id", http.StatusBadRequest)
		return
	}

	isLocal := peerID == "local"
	if s.node != nil && peerID == s.node.PeerID().String() {
		isLocal = true
	}

	if isLocal {
		content, err := s.store.GetPlanContent(planID)
		if err != nil {
			http.Error(w, "plan not found", http.StatusNotFound)
			return
		}
		plan, _ := s.store.GetPlan(planID)
		writeJSON(w, map[string]any{
			"peer_name": s.peerName,
			"peer_id":   peerID,
			"plan_id":   planID,
			"summary":   plan.Summary,
			"content":   content,
		})
		return
	}

	if s.node == nil {
		http.Error(w, "P2P not initialized", http.StatusServiceUnavailable)
		return
	}

	pid, err := peer.Decode(peerID)
	if err != nil {
		http.Error(w, "invalid peer_id", http.StatusBadRequest)
		return
	}

	resp, err := protocol.QueryPeerGet(r.Context(), s.node.Host, pid, planID, s.timeout)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, resp.Error, http.StatusNotFound)
		return
	}
	writeJSON(w, resp)
}

func (s *Server) handleListPeers(w http.ResponseWriter, r *http.Request) {
	type peerInfo struct {
		PeerID    string   `json:"peer_id"`
		Addrs     []string `json:"addrs"`
		Connected bool     `json:"connected"`
	}

	if s.node == nil {
		writeJSON(w, []any{})
		return
	}

	peers := s.node.Peers()
	infos := make([]peerInfo, 0, len(peers))
	for _, pid := range peers {
		addrs := s.node.Host.Peerstore().Addrs(pid)
		addrStrs := make([]string, len(addrs))
		for i, a := range addrs {
			addrStrs[i] = a.String()
		}
		infos = append(infos, peerInfo{
			PeerID:    pid.String(),
			Addrs:     addrStrs,
			Connected: s.node.IsConnected(pid),
		})
	}
	writeJSON(w, infos)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	plans := s.store.ListPlans()

	peerID := "local"
	peerCount := 0
	if s.node != nil {
		peerID = s.node.PeerID().String()
		peerCount = len(s.node.Peers())
	}

	writeJSON(w, map[string]any{
		"status":     "ok",
		"peer_name":  s.peerName,
		"peer_id":    peerID,
		"plan_count": len(plans),
		"peer_count": peerCount,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func isValidPlanID(id string) bool {
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return len(id) > 0
}
