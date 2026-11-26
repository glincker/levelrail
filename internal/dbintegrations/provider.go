package dbintegrations

import (
	"context"
	"errors"
)

// StateCode is the per card state shown in the dashboard and CLI.
type StateCode string

// The card states.
const (
	StateConnected   StateCode = "connected"
	StateAvailable   StateCode = "available"
	StateNeedsSetup  StateCode = "needs_setup"
	StateDocsOnly    StateCode = "docs_only"
	StateUnavailable StateCode = "unavailable"
	StatePlanned     StateCode = "planned"
)

// State is a card's derived status with a short human reason.
type State struct {
	Code   StateCode `json:"code"`
	Reason string    `json:"reason,omitempty"`
}

// Database identifies the database a card is evaluated for.
type Database struct {
	Name   string
	Engine string
}

// SecretRef points at a stored secret by name, so a config never holds a value.
type SecretRef struct {
	Name string `json:"name"`
}

// Config is the input of Configure: plain settings plus secrets by reference.
type Config struct {
	Settings map[string]string    `json:"settings,omitempty"`
	Secrets  map[string]SecretRef `json:"secrets,omitempty"`
}

// ErrReadOnly is returned by providers that only report status.
var ErrReadOnly = errors.New("dbintegrations: provider is read only")

// Provider derives a card's status and, when it is a writable connector,
// applies its configuration.
type Provider interface {
	ID() string
	Status(ctx context.Context, db Database) (State, error)
	Configure(ctx context.Context, db Database, cfg Config) error
}
