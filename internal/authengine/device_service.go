package authengine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
)

// Device grant outcomes, in the vocabulary the platform's device routes already use.
var (
	ErrDevicePending   = errors.New("authengine: authorization_pending")
	ErrDeviceDenied    = errors.New("authengine: access_denied")
	ErrDeviceExpired   = errors.New("authengine: expired_token")
	ErrDeviceNotFound  = errors.New("authengine: no pending device login with this code")
	ErrDeviceThrottled = errors.New("authengine: too many failed device code attempts")
	ErrDeviceAbilities = errors.New("authengine: approver holds none of the requested abilities")
)

// DeviceStartInput describes the requesting client.
type DeviceStartInput struct {
	ClientName string
	IP         string
	UserAgent  string
}

// DeviceStartResult is the new request's codes and timing.
type DeviceStartResult struct {
	DeviceCode string
	UserCode   string
	ExpiresIn  time.Duration
	Interval   time.Duration
}

// PendingDevice is one request awaiting approval, as the dashboard lists it.
type PendingDevice struct {
	UserCode   string
	ClientName string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// DeviceDecision is an approver's verdict. Abilities are legacy names already
// capped by the caller; a lone root is honored only for an approver who holds it.
type DeviceDecision struct {
	ApproverLegacyID string
	IP               string
	UserCode         string
	Approve          bool
	Abilities        []string
}

// StartDevice opens a request that may be approved for any non-root ability.
func (e *Engine) StartDevice(ctx context.Context, in DeviceStartInput) (DeviceStartResult, error) {
	s, err := e.auth.StartDeviceAuth(ctx, theauth.StartDeviceAuthInput{
		ClientName: in.ClientName, Abilities: EngineAbilities(), IP: in.IP, UserAgent: in.UserAgent,
	})
	if err != nil {
		return DeviceStartResult{}, fmt.Errorf("authengine: start device login: %w", err)
	}
	return DeviceStartResult{DeviceCode: s.DeviceCode, UserCode: s.UserCode, ExpiresIn: s.ExpiresIn, Interval: s.Interval}, nil
}

// RedeemDevice exchanges an approved code for a token.
// Polling too fast reports pending, matching how the platform's clients poll.
func (e *Engine) RedeemDevice(ctx context.Context, deviceCode string) (string, TokenRecord, error) {
	if e.deviceRedeemed(ctx, deviceCode) {
		return "", TokenRecord{}, ErrDeviceExpired
	}
	tok, err := e.auth.RedeemDeviceCode(ctx, deviceCode)
	switch {
	case errors.Is(err, theauth.ErrDeviceAuthorizationPending), errors.Is(err, theauth.ErrDeviceSlowDown):
		return "", TokenRecord{}, ErrDevicePending
	case errors.Is(err, theauth.ErrDeviceDenied):
		return "", TokenRecord{}, ErrDeviceDenied
	case errors.Is(err, theauth.ErrDeviceExpired), errors.Is(err, theauth.ErrDeviceInvalid):
		return "", TokenRecord{}, ErrDeviceExpired
	case err != nil:
		return "", TokenRecord{}, fmt.Errorf("authengine: redeem device code: %w", err)
	}
	rec, err := e.toRecord(ctx, tok.APIToken)
	if err != nil {
		return "", TokenRecord{}, err
	}
	return tok.Token, rec, nil
}

// deviceRedeemed reports an already-redeemed code up front: the library would
// answer slow-down (pending) inside the poll interval, the legacy flow says expired.
func (e *Engine) deviceRedeemed(ctx context.Context, deviceCode string) bool {
	hash := sha256.Sum256([]byte(deviceCode))
	rec, err := e.store.DeviceCodeByHash(ctx, hash[:])
	return err == nil && rec != nil && rec.Status == theauth.DeviceStatusRedeemed
}

// DecideDevice approves or denies a pending request.
func (e *Engine) DecideDevice(ctx context.Context, d DeviceDecision) error {
	engineID, err := e.dir.EnsureEngineUser(ctx, d.ApproverLegacyID)
	if err != nil {
		return err
	}
	approverID, err := parseULID(engineID)
	if err != nil {
		return err
	}
	approver, err := e.store.UserByID(ctx, approverID)
	if err != nil {
		return fmt.Errorf("authengine: load approver: %w", err)
	}
	if d.Approve && len(d.Abilities) == 1 && d.Abilities[0] == AbilityRoot {
		return e.approveRoot(ctx, approver, d)
	}
	abilities, err := MapAbilities(d.Abilities)
	if err != nil {
		return err
	}
	if !d.Approve {
		abilities = nil
	}
	return mapDecideErr(e.auth.DecideDeviceRequest(ctx, approver, d.IP, d.UserCode, d.Approve, abilities))
}

// approveRoot grants a lone root, which the library's request set cannot
// carry (root is exclusive), after the library has vetted the request.
func (e *Engine) approveRoot(ctx context.Context, approver *theauth.User, d DeviceDecision) error {
	held, err := e.dir.UserAbilities(ctx, approver)
	if err != nil {
		return err
	}
	if !containsString(held, theauth.AbilityRoot) {
		return ErrDeviceAbilities
	}
	if _, err := e.auth.LookupDeviceRequest(ctx, approver, d.IP, d.UserCode); err != nil {
		return mapDecideErr(err)
	}
	err = e.store.DecideDeviceCode(ctx, normalizeCode(d.UserCode), theauth.DeviceDecision{
		Approve: true, ApproverID: approver.ID, Abilities: []string{theauth.AbilityRoot},
	}, time.Now().UTC())
	if errors.Is(err, theauth.ErrStorageNotFound) {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("authengine: record device decision: %w", err)
	}
	return nil
}

func mapDecideErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, theauth.ErrDeviceInvalid), errors.Is(err, theauth.ErrDeviceExpired):
		return ErrDeviceNotFound
	case errors.Is(err, theauth.ErrDeviceAttemptsExceeded):
		return ErrDeviceThrottled
	case errors.Is(err, theauth.ErrAbilityNotHeld):
		return ErrDeviceAbilities
	}
	return fmt.Errorf("authengine: decide device request: %w", err)
}

// PendingDevices lists unexpired requests awaiting a decision, newest first.
func (e *Engine) PendingDevices(ctx context.Context) ([]PendingDevice, error) {
	rows, err := e.store.ListPendingDeviceCodes(ctx, theauth.DevicePendingFilter{Now: time.Now().UTC(), Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("authengine: list pending device logins: %w", err)
	}
	out := make([]PendingDevice, 0, len(rows))
	for _, r := range rows {
		out = append(out, PendingDevice{
			UserCode: theauth.FormatUserCode(r.UserCode), ClientName: r.ClientName, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt,
		})
	}
	return out, nil
}
