package api

import (
	"context"
	"path/filepath"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/imagemove"
)

// Image move states.
const (
	imageMovePending   = "pending"
	imageMoveRunning   = "running"
	imageMoveVerified  = "verified"
	imageMoveLoaded    = "loaded"
	imageMoveFailed    = "failed"
	imageMoveCancelled = "cancelled"
)

const (
	envImageMoveMaxBytes    = "APP_MIGRATE_IMAGE_MAX_BYTES"
	envImageMoveTimeout     = "APP_MIGRATE_IMAGE_TIMEOUT"
	defaultImageMoveMax     = int64(20) << 30
	defaultImageMoveTimeout = time.Hour
	imageMoveKnownHosts     = "import-known-hosts"
)

type appImportImage struct {
	SourceID      string `json:"source_id"`
	App           string `json:"app"`
	Target        string `json:"target,omitempty"`
	Image         string `json:"image"`
	SourceImageID string `json:"source_image_id,omitempty"`
	LoadedImageID string `json:"loaded_image_id,omitempty"`
	Node          string `json:"node,omitempty"`
	State         string `json:"state"`
	Bytes         int64  `json:"bytes"`
	Verified      bool   `json:"verified"`
	Error         string `json:"error,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type appImportImagesResource struct {
	Running         bool             `json:"running"`
	Source          string           `json:"source,omitempty"`
	CredentialsHeld bool             `json:"credentials_held"`
	Supported       bool             `json:"supported"`
	MaxBytes        int64            `json:"max_bytes"`
	Images          []appImportImage `json:"images"`
}

// imageMoveRun is one session's image move, in memory only: the SSH
// credential never reaches the store or a log line.
type imageMoveRun struct {
	cancel  context.CancelFunc
	running bool
	target  imagemove.Target
	creds   *imagemove.Credentials
	expires time.Time
	images  map[string]*appImportImage
}

type saverFactory func(t imagemove.Target, c imagemove.Credentials, keys *imagemove.HostKeys) imagemove.Saver

func defaultSaverFactory(t imagemove.Target, c imagemove.Credentials, keys *imagemove.HostKeys) imagemove.Saver {
	return &imagemove.SSHSaver{Target: t, Creds: c, HostKeys: keys}
}

func (s *appImportState) moveRun(id string) *imageMoveRun {
	if s.moves == nil {
		s.moves = map[string]*imageMoveRun{}
	}
	m, ok := s.moves[id]
	if !ok {
		m = &imageMoveRun{images: map[string]*appImportImage{}}
		s.moves[id] = m
	}
	if m.creds != nil && !m.running && time.Now().After(m.expires) {
		m.creds = nil
	}
	return m
}

func (s *appImportState) hostKeyStore(dataDir string) *imagemove.HostKeys {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hostKeys == nil {
		path := ""
		if dataDir != "" {
			path = filepath.Join(dataDir, imageMoveKnownHosts)
		}
		s.hostKeys = &imagemove.HostKeys{Path: path}
	}
	return s.hostKeys
}

func (s *appImportState) saverFor(t imagemove.Target, c imagemove.Credentials, keys *imagemove.HostKeys) imagemove.Saver {
	s.mu.Lock()
	f := s.newSaver
	s.mu.Unlock()
	if f == nil {
		f = defaultSaverFactory
	}
	return f(t, c, keys)
}

// startMove claims the session's move, returning false when one runs.
func (s *appImportState) startMove(id string, t imagemove.Target, c imagemove.Credentials, cancel context.CancelFunc, queued []appImportImage) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.moveRun(id)
	if m.running {
		return false
	}
	m.running, m.cancel, m.target = true, cancel, t
	creds := c
	m.creds = &creds
	m.expires = time.Now().Add(envDuration(envHubPasswordTTL, 2*time.Hour))
	for _, img := range queued {
		img := img
		m.images[img.SourceID] = &img
	}
	return true
}

func (s *appImportState) heldCreds(id string) (imagemove.Target, *imagemove.Credentials) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.moveRun(id)
	return m.target, m.creds
}

func (s *appImportState) updateImage(id, sourceID string, fn func(*appImportImage)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if img, ok := s.moveRun(id).images[sourceID]; ok {
		fn(img)
		img.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func (s *appImportState) finishMove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.moveRun(id)
	m.running, m.cancel = false, nil
	for _, img := range m.images {
		if img.State == imageMovePending || img.State == imageMoveRunning {
			img.State, img.Error = imageMoveCancelled, "the move was cancelled before this image finished"
		}
	}
}

func (s *appImportState) cancelMove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.moveRun(id)
	if !m.running || m.cancel == nil {
		return false
	}
	m.cancel()
	return true
}

func (s *appImportState) forgetMove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.moves[id]; ok {
		if m.cancel != nil {
			m.cancel()
		}
		delete(s.moves, id)
	}
}

// moveSnapshot copies the in-memory state for a response.
func (s *appImportState) moveSnapshot(id string) (running bool, source string, held bool, imgs map[string]appImportImage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.moveRun(id)
	imgs = map[string]appImportImage{}
	for k, v := range m.images {
		imgs[k] = *v
	}
	if m.target.Host != "" {
		source = m.target.String()
	}
	return m.running, source, m.creds != nil, imgs
}

func sortImages(list []appImportImage) {
	sort.Slice(list, func(i, j int) bool { return list[i].App < list[j].App })
}
