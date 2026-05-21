package planstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func writePlan(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestScanDirectory(t *testing.T) {
	dir := setupTestDir(t)
	writePlan(t, dir, "test-plan.md", "# My Test Plan\n\n## Context\nSome context.")
	writePlan(t, dir, "another.md", "# Another Plan\n\nDetails.")

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := store.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	plans := store.ListPlans()
	if len(plans) != 2 {
		t.Fatalf("plan count = %d, want 2", len(plans))
	}
}

func TestScanDirectorySkipsNonMarkdown(t *testing.T) {
	dir := setupTestDir(t)
	writePlan(t, dir, "plan.md", "# Real Plan")
	writePlan(t, dir, "notes.txt", "not a plan")
	writePlan(t, dir, "data.json", "{}")

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	plans := store.ListPlans()
	if len(plans) != 1 {
		t.Fatalf("plan count = %d, want 1 (only .md files)", len(plans))
	}
}

func TestScanDirectoryCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent", "plans")
	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := store.Watch(ctx); err != nil {
		t.Fatalf("Watch should create dir: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("directory should have been created")
	}
}

func TestExtractSummary(t *testing.T) {
	dir := setupTestDir(t)

	tests := []struct {
		filename string
		content  string
		want     string
	}{
		{"h1.md", "# Simple Title\n\nBody.", "Simple Title"},
		{"plan-prefix.md", "# Plan: Create Something\n\nBody.", "Create Something"},
		{"empty-lines.md", "\n\n# Title After Blanks\n\nBody.", "Title After Blanks"},
		{"no-heading.md", "First line is not a heading\n\nMore text.", "First line is not a heading"},
		{"empty.md", "", "Empty"},
	}

	for _, tt := range tests {
		writePlan(t, dir, tt.filename, tt.content)
	}

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	for _, tt := range tests {
		id := planID(filepath.Join(dir, tt.filename))
		plan, ok := store.GetPlan(id)
		if !ok {
			t.Errorf("%s: plan not found", tt.filename)
			continue
		}
		if plan.Summary != tt.want {
			t.Errorf("%s: summary = %q, want %q", tt.filename, plan.Summary, tt.want)
		}
	}
}

func TestGetPlanContent(t *testing.T) {
	dir := setupTestDir(t)
	content := "# Test Plan\n\n## Context\n\nFull content here."
	writePlan(t, dir, "readable.md", content)

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	got, err := store.GetPlanContent("readable")
	if err != nil {
		t.Fatalf("GetPlanContent: %v", err)
	}
	if got != content {
		t.Errorf("content mismatch: got %d bytes, want %d bytes", len(got), len(content))
	}
}

func TestGetPlanContentNotFound(t *testing.T) {
	dir := setupTestDir(t)
	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	_, err := store.GetPlanContent("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plan")
	}
}

// --- Path traversal tests ---

func TestGetPlanContentPathTraversal(t *testing.T) {
	dir := setupTestDir(t)
	writePlan(t, dir, "legit.md", "# Legit Plan")

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	attacks := []string{
		"../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"....//....//etc//passwd",
		"../legit",
		"./legit",
		"/etc/passwd",
		"legit/../../../etc/passwd",
		"..",
		".",
		"",
	}

	for _, id := range attacks {
		content, err := store.GetPlanContent(id)
		if err == nil && content != "" {
			t.Errorf("path traversal should fail for %q, got content (%d bytes)", id, len(content))
		}
	}
}

func TestGetPlanContentSymlinkEscape(t *testing.T) {
	dir := setupTestDir(t)

	secretDir := t.TempDir()
	secretFile := filepath.Join(secretDir, "secret.txt")
	os.WriteFile(secretFile, []byte("TOP SECRET"), 0644)

	symlink := filepath.Join(dir, "evil.md")
	if err := os.Symlink(secretFile, symlink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	_, err := store.GetPlanContent("evil")
	if err == nil {
		t.Fatal("symlink escape should be blocked")
	}
}

func TestGetPlanContentSymlinkWithinDir(t *testing.T) {
	dir := setupTestDir(t)
	writePlan(t, dir, "real.md", "# Real Plan")

	symlink := filepath.Join(dir, "alias.md")
	if err := os.Symlink(filepath.Join(dir, "real.md"), symlink); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	content, err := store.GetPlanContent("alias")
	if err != nil {
		t.Fatalf("symlink within dir should be allowed: %v", err)
	}
	if content != "# Real Plan" {
		t.Errorf("unexpected content: %q", content)
	}
}

// --- fsnotify tests ---

func TestFsnotifyNewFile(t *testing.T) {
	dir := setupTestDir(t)
	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	if len(store.ListPlans()) != 0 {
		t.Fatal("should start empty")
	}

	writePlan(t, dir, "new-plan.md", "# New Plan")
	time.Sleep(200 * time.Millisecond)

	plans := store.ListPlans()
	if len(plans) != 1 {
		t.Fatalf("plan count = %d, want 1 after adding file", len(plans))
	}
	if plans[0].Summary != "New Plan" {
		t.Errorf("summary = %q, want %q", plans[0].Summary, "New Plan")
	}
}

func TestFsnotifyDeleteFile(t *testing.T) {
	dir := setupTestDir(t)
	writePlan(t, dir, "temp.md", "# Temporary Plan")

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	if len(store.ListPlans()) != 1 {
		t.Fatal("should have 1 plan")
	}

	os.Remove(filepath.Join(dir, "temp.md"))
	time.Sleep(200 * time.Millisecond)

	if len(store.ListPlans()) != 0 {
		t.Fatal("plan should be removed after file deletion")
	}
}

func TestFsnotifyUpdateFile(t *testing.T) {
	dir := setupTestDir(t)
	writePlan(t, dir, "evolving.md", "# Version 1")

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	plan, _ := store.GetPlan("evolving")
	if plan.Summary != "Version 1" {
		t.Fatalf("initial summary = %q", plan.Summary)
	}

	writePlan(t, dir, "evolving.md", "# Version 2")
	time.Sleep(200 * time.Millisecond)

	plan, _ = store.GetPlan("evolving")
	if plan.Summary != "Version 2" {
		t.Errorf("updated summary = %q, want %q", plan.Summary, "Version 2")
	}
}

// --- planID tests ---

func TestPlanID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/.claude/plans/my-plan.md", "my-plan"},
		{"/tmp/test/plan-with-dots.v2.md", "plan-with-dots.v2"},
		{"simple.md", "simple"},
	}
	for _, tt := range tests {
		got := planID(tt.path)
		if got != tt.want {
			t.Errorf("planID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestHumanizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-cool-plan.md", "My cool plan"},
		{"under_score_name.md", "Under score name"},
		{"simple.md", "Simple"},
	}
	for _, tt := range tests {
		got := humanizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("humanizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Binary file test ---

func TestSkipBinaryFiles(t *testing.T) {
	dir := setupTestDir(t)
	os.WriteFile(filepath.Join(dir, "binary.md"), []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}, 0644)
	writePlan(t, dir, "text.md", "# Text Plan")

	store := New(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Watch(ctx)

	plans := store.ListPlans()
	if len(plans) != 1 {
		t.Fatalf("plan count = %d, want 1 (binary should be skipped)", len(plans))
	}
}
