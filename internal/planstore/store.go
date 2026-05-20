package planstore

import (
	"bufio"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
)

type Plan struct {
	ID       string    `json:"id"`
	Summary  string    `json:"summary"`
	Modified time.Time `json:"modified"`
	FilePath string    `json:"-"`
}

type Store struct {
	dir     string
	mu      sync.RWMutex
	plans   map[string]*Plan
	watcher *fsnotify.Watcher
}

func New(dir string) *Store {
	return &Store{
		dir:   dir,
		plans: make(map[string]*Plan),
	}
}

func (s *Store) Watch(ctx context.Context) error {
	os.MkdirAll(s.dir, 0755)

	s.scanDirectory()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.watcher = watcher

	if err := watcher.Add(s.dir); err != nil {
		watcher.Close()
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				watcher.Close()
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !strings.HasSuffix(event.Name, ".md") {
					continue
				}
				switch {
				case event.Has(fsnotify.Create) || event.Has(fsnotify.Write):
					s.indexFile(event.Name)
				case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
					s.removeFile(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("planstore: watcher error: %v", err)
			}
		}
	}()
	return nil
}

func (s *Store) ListPlans() []Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	plans := make([]Plan, 0, len(s.plans))
	for _, p := range s.plans {
		plans = append(plans, *p)
	}
	return plans
}

func (s *Store) GetPlan(id string) (*Plan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

func (s *Store) GetPlanContent(id string) (string, error) {
	s.mu.RLock()
	p, ok := s.plans[id]
	s.mu.RUnlock()
	if !ok {
		return "", os.ErrNotExist
	}
	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) scanDirectory() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		log.Printf("planstore: scan error: %v", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		s.indexFile(filepath.Join(s.dir, entry.Name()))
	}
}

func (s *Store) indexFile(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	summary, err := extractSummary(path)
	if err != nil {
		log.Printf("planstore: skip %s: %v", filepath.Base(path), err)
		return
	}

	id := planID(path)
	s.mu.Lock()
	s.plans[id] = &Plan{
		ID:       id,
		Summary:  summary,
		Modified: info.ModTime(),
		FilePath: path,
	}
	s.mu.Unlock()
}

func (s *Store) removeFile(path string) {
	id := planID(path)
	s.mu.Lock()
	delete(s.plans, id)
	s.mu.Unlock()
}

func planID(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

func extractSummary(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n > 0 && !utf8.Valid(buf[:n]) {
		return "", os.ErrInvalid
	}
	f.Seek(0, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			summary := strings.TrimPrefix(line, "# ")
			summary = strings.TrimPrefix(summary, "Plan: ")
			return summary, nil
		}
		return line, nil
	}

	return humanizeFilename(filepath.Base(filePath)), nil
}

func humanizeFilename(name string) string {
	name = strings.TrimSuffix(name, ".md")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}
