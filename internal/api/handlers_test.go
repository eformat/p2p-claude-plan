package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/p2p-claude-plans/p2p-claude-plans/internal/config"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/planstore"
)

func setupTestServer(t *testing.T) (*Server, *planstore.Store) {
	t.Helper()
	dir := t.TempDir()

	writeTestPlan(t, dir, "test-plan.md", "# Test Plan\n\n## Context\nTest content.")
	writeTestPlan(t, dir, "another.md", "# Another Plan\n\nMore content.")

	store := planstore.New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := store.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	cfg := &config.Config{
		PeerName:       "test-peer",
		HTTPPort:       0,
		RequestTimeout: 5,
	}
	srv := NewServer(store, cfg)
	return srv, store
}

func writeTestPlan(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// --- Health endpoint tests ---

func TestHealthEndpoint(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	if resp["peer_name"] != "test-peer" {
		t.Errorf("peer_name = %v, want test-peer", resp["peer_name"])
	}
	planCount := int(resp["plan_count"].(float64))
	if planCount != 2 {
		t.Errorf("plan_count = %d, want 2", planCount)
	}
}

// --- Plans endpoint tests ---

func TestListPlansLocal(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/plans", nil)
	w := httptest.NewRecorder()
	srv.handleListPlans(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp []peerPlansJSON
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp) != 1 {
		t.Fatalf("peer count = %d, want 1", len(resp))
	}
	if !resp[0].IsLocal {
		t.Error("should be marked as local")
	}
	if len(resp[0].Plans) != 2 {
		t.Errorf("plan count = %d, want 2", len(resp[0].Plans))
	}
}

// --- GetPlan endpoint tests ---

func TestGetPlanLocal(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/plans/local/test-plan", nil)
	req.SetPathValue("peerID", "local")
	req.SetPathValue("planID", "test-plan")
	w := httptest.NewRecorder()
	srv.handleGetPlan(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["plan_id"] != "test-plan" {
		t.Errorf("plan_id = %v", resp["plan_id"])
	}
	content := resp["content"].(string)
	if len(content) == 0 {
		t.Error("content should not be empty")
	}
}

func TestGetPlanNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/plans/local/nonexistent", nil)
	req.SetPathValue("peerID", "local")
	req.SetPathValue("planID", "nonexistent")
	w := httptest.NewRecorder()
	srv.handleGetPlan(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- Path traversal tests on HTTP API ---

func TestGetPlanPathTraversal(t *testing.T) {
	srv, _ := setupTestServer(t)

	attacks := []string{
		"../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"test-plan/../../../etc/passwd",
		"test-plan/../../secret",
		"..",
		".",
	}

	for _, id := range attacks {
		req := httptest.NewRequest(http.MethodGet, "/plans/local/"+id, nil)
		req.SetPathValue("peerID", "local")
		req.SetPathValue("planID", id)
		w := httptest.NewRecorder()
		srv.handleGetPlan(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("path traversal should fail for %q, got 200", id)
		}
	}
}

func TestGetPlanInjectionAttempts(t *testing.T) {
	srv, _ := setupTestServer(t)

	injections := []string{
		"plan; rm -rf /",
		"plan$(whoami)",
		"plan`id`",
		"plan|cat /etc/passwd",
		"plan\x00null",
		"{\"type\":\"list\"}",
		"<script>alert(1)</script>",
	}

	for _, id := range injections {
		req := httptest.NewRequest(http.MethodGet, "/plans/local/x", nil)
		req.SetPathValue("peerID", "local")
		req.SetPathValue("planID", id)
		w := httptest.NewRecorder()
		srv.handleGetPlan(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("injection should fail for %q, got 200", id)
		}
	}
}

// --- isValidPlanID tests ---

func TestIsValidPlanID(t *testing.T) {
	valid := []string{
		"my-plan",
		"plan_v2",
		"plan.with.dots",
		"CamelCase123",
	}
	for _, id := range valid {
		if !isValidPlanID(id) {
			t.Errorf("expected valid: %q", id)
		}
	}

	invalid := []string{
		"",
		"../etc/passwd",
		"plan with spaces",
		"plan/subdir",
		"plan\\backslash",
		"plan\x00null",
	}
	for _, id := range invalid {
		if isValidPlanID(id) {
			t.Errorf("expected invalid: %q", id)
		}
	}
}

// --- Peers endpoint tests ---

func TestListPeersNoNode(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/peers", nil)
	w := httptest.NewRecorder()
	srv.handleListPeers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp []any
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("peers = %d, want 0 (no node)", len(resp))
	}
}

func TestGetPlanRemoteNoNode(t *testing.T) {
	srv, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/plans/12D3KooWFake/some-plan", nil)
	req.SetPathValue("peerID", "12D3KooWFake")
	req.SetPathValue("planID", "some-plan")
	w := httptest.NewRecorder()
	srv.handleGetPlan(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (no P2P node)", w.Code)
	}
}
