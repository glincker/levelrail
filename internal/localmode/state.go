package localmode

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	adminUsername      = "admin"
	adminPasswordBytes = 18
)

// Admin is the generated sign-in for a local instance.
type Admin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoadOrCreateAdmin reads the stored admin or generates one; created is true only for a new credential.
func LoadOrCreateAdmin(dataDir string, random io.Reader) (a Admin, created bool, err error) {
	path := AdminFilePath(dataDir)
	raw, err := os.ReadFile(path) //nolint:gosec // path is under the operator's own data dir
	if err == nil {
		if jerr := json.Unmarshal(raw, &a); jerr == nil && a.Password != "" {
			return a, false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Admin{}, false, fmt.Errorf("localmode: read admin file: %w", err)
	}
	if random == nil {
		random = rand.Reader
	}
	buf := make([]byte, adminPasswordBytes)
	if _, err := io.ReadFull(random, buf); err != nil {
		return Admin{}, false, fmt.Errorf("localmode: generate admin password: %w", err)
	}
	a = Admin{Username: adminUsername, Password: base64.RawURLEncoding.EncodeToString(buf)}
	out, err := json.Marshal(a) //nolint:gosec // the credential file is the point: operator-only, mode 0600
	if err != nil {
		return Admin{}, false, fmt.Errorf("localmode: encode admin: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return Admin{}, false, fmt.Errorf("localmode: write admin file: %w", err)
	}
	return a, true, nil
}

// State is what a running supervisor records so stop and status can find it.
type State struct {
	PID       int       `json:"pid"`
	DataDir   string    `json:"data_dir"`
	Dashboard string    `json:"dashboard_url"`
	HTTPPort  int       `json:"http_port"`
	HTTPSPort int       `json:"https_port"`
	Started   time.Time `json:"started"`
}

// WriteState records st in its data dir.
func WriteState(st State) error {
	out, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("localmode: encode state: %w", err)
	}
	if err := os.WriteFile(StateFilePath(st.DataDir), out, 0o600); err != nil {
		return fmt.Errorf("localmode: write state: %w", err)
	}
	return nil
}

// ReadState loads the state recorded in dataDir; ok is false when none exists.
func ReadState(dataDir string) (st State, ok bool, err error) {
	raw, err := os.ReadFile(StateFilePath(dataDir)) //nolint:gosec // path is under the operator's own data dir
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("localmode: read state: %w", err)
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, false, fmt.Errorf("localmode: decode state: %w", err)
	}
	return st, true, nil
}

// ClearState removes the state file.
func ClearState(dataDir string) {
	_ = os.Remove(StateFilePath(dataDir))
}
