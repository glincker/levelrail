package main

import (
	"os"
	"strconv"
	"time"
)

// Wire types. Times stay strings so the signed bytes never depend on a
// time formatting round trip.
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type Localized struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Item struct {
	ID            string               `json:"id"`
	Tier          string               `json:"tier"`
	Kind          string               `json:"kind"`
	Title         string               `json:"title"`
	Body          string               `json:"body"`
	AppliesTo     string               `json:"applies_to,omitempty"`
	Channels      []string             `json:"channels,omitempty"`
	RequiresFlags []string             `json:"requires_flags,omitempty"`
	CTA           string               `json:"cta,omitempty"`
	Links         []Link               `json:"links,omitempty"`
	I18n          map[string]Localized `json:"i18n,omitempty"`
	RevokeKeyID   string               `json:"revoke_key_id,omitempty"`
	IssuedAt      string               `json:"issued_at"`
	ExpiresAt     string               `json:"expires_at"`
}

type Feed struct {
	Schema      int    `json:"schema"`
	Sequence    int64  `json:"sequence"`
	GeneratedAt string `json:"generated_at"`
	ExpiresAt   string `json:"expires_at"`
	Items       []Item `json:"items"`
}

// Limits are the caps shared by the validator and the verifier. Defaults
// match the instance seeds; env overrides keep them out of the code.
type Limits struct {
	MaxBytes    int
	MaxItems    int
	TitleRunes  int
	BodyRunes   int
	MaxLifetime time.Duration
	Skew        time.Duration
	FeedMaxAge  time.Duration
}

func defaultLimits() Limits {
	day := 24 * time.Hour
	return Limits{
		MaxBytes:    envInt("ANNOUNCE_MAX_BYTES", 256*1024),
		MaxItems:    envInt("ANNOUNCE_MAX_ITEMS", 50),
		TitleRunes:  envInt("ANNOUNCE_TITLE_RUNES", 120),
		BodyRunes:   envInt("ANNOUNCE_BODY_RUNES", 2000),
		MaxLifetime: time.Duration(envInt("ANNOUNCE_MAX_LIFETIME_DAYS", 366)) * day,
		Skew:        time.Duration(envInt("ANNOUNCE_CLOCK_SKEW_HOURS", 24)) * time.Hour,
		FeedMaxAge:  time.Duration(envInt("ANNOUNCE_FEED_MAX_AGE_DAYS", 30)) * day,
	}
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// sigsNeeded is how many distinct keys must sign an item of this tier.
var sigsNeeded = map[string]int{
	"critical": 2, "advisory": 1, "feature": 1, "maintenance": 1, "info": 1, "newsletter": 1,
}
