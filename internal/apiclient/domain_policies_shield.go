package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// ShieldLimits are the instance caps and under-attack defaults for the abuse shield.
type ShieldLimits struct {
	MaxBotAgents           int `json:"max_bot_agents"`
	MaxRetryAfterSeconds   int `json:"max_retry_after_seconds"`
	DefaultRetryAfter      int `json:"default_retry_after_seconds"`
	TarpitMaxMillis        int `json:"tarpit_max_ms"`
	AttackRatePerMinute    int `json:"attack_rate_per_minute"`
	AttackBurst            int `json:"attack_burst"`
	AttackMaxConcurrent    int `json:"attack_max_concurrent"`
	AttackDefaultMinutes   int `json:"attack_default_minutes"`
	AttackMaxMinutes       int `json:"attack_max_minutes"`
	MaxTrackedIPsPerDomain int `json:"max_tracked_ips"`
}

// ShieldPolicyResource is GET/PUT .../domains/{domain}/shield.
type ShieldPolicyResource struct {
	Domain     string               `json:"domain"`
	Kind       string               `json:"kind"`
	Configured bool                 `json:"configured"`
	Spec       trafficpolicy.Shield `json:"spec"`
	UpdatedAt  string               `json:"updated_at,omitempty"`
}

// ShieldBotList is GET /api/v1/system/shield/bots.
type ShieldBotList struct {
	Agents []string `json:"agents"`
	Custom bool     `json:"custom"`
}

// UnderAttackRequest is the body of POST .../shield/under-attack.
type UnderAttackRequest struct {
	Minutes int `json:"minutes,omitempty"`
}

// GetDomainShield calls GET .../domains/{domain}/shield.
func (c *Client) GetDomainShield(ctx context.Context, name, domain string) (ShieldPolicyResource, error) {
	var out ShieldPolicyResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, trafficpolicy.KindShield), nil, &out)
	return out, err
}

// SetDomainShield calls PUT .../domains/{domain}/shield.
func (c *Client) SetDomainShield(ctx context.Context, name, domain string, spec trafficpolicy.Shield) (ShieldPolicyResource, error) {
	var out ShieldPolicyResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, trafficpolicy.KindShield), spec, &out)
	return out, err
}

// ClearDomainShield calls DELETE .../domains/{domain}/shield.
func (c *Client) ClearDomainShield(ctx context.Context, name, domain string) (ShieldPolicyResource, error) {
	var out ShieldPolicyResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, trafficpolicy.KindShield), nil, &out)
	return out, err
}

// StartUnderAttack calls POST .../domains/{domain}/shield/under-attack.
func (c *Client) StartUnderAttack(ctx context.Context, name, domain string, minutes int) (ShieldPolicyResource, error) {
	var out ShieldPolicyResource
	err := c.do(ctx, http.MethodPost, domainPolicyPath(name, domain, trafficpolicy.KindShield+"/under-attack"), UnderAttackRequest{Minutes: minutes}, &out)
	return out, err
}

// StopUnderAttack calls DELETE .../domains/{domain}/shield/under-attack.
func (c *Client) StopUnderAttack(ctx context.Context, name, domain string) (ShieldPolicyResource, error) {
	var out ShieldPolicyResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, trafficpolicy.KindShield+"/under-attack"), nil, &out)
	return out, err
}

// ShieldBots calls GET /api/v1/system/shield/bots.
func (c *Client) ShieldBots(ctx context.Context) (ShieldBotList, error) {
	var out ShieldBotList
	err := c.do(ctx, http.MethodGet, "/api/v1/system/shield/bots", nil, &out)
	return out, err
}
