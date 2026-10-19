// Package search is the unified search service: a SQLite FTS5 index fed by
// providers, filtered through an Authorizer before any hit leaves it.
package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind is the category of a search document.
type Kind string

// Document kinds.
const (
	KindApp        Kind = "app"
	KindDatabase   Kind = "database"
	KindNode       Kind = "node"
	KindDomain     Kind = "domain"
	KindTemplate   Kind = "template"
	KindProject    Kind = "project"
	KindEnv        Kind = "environment"
	KindTag        Kind = "tag"
	KindDeployment Kind = "deployment"
	KindSetting    Kind = "setting"
	KindDoc        Kind = "doc"
	KindAction     Kind = "action"
	KindSaved      Kind = "saved"
)

var kindAliases = map[string]Kind{
	"db": KindDatabase, "databases": KindDatabase, "apps": KindApp, "nodes": KindNode,
	"domains": KindDomain, "tpl": KindTemplate, "templates": KindTemplate,
	"deploy": KindDeployment, "deployments": KindDeployment, "settings": KindSetting,
	"docs": KindDoc, "actions": KindAction, "env": KindEnv, "environments": KindEnv,
	"projects": KindProject, "tags": KindTag,
}

// ParseKind resolves a kind name or alias.
func ParseKind(s string) (Kind, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch k := Kind(s); k {
	case KindApp, KindDatabase, KindNode, KindDomain, KindTemplate, KindProject, KindEnv,
		KindTag, KindDeployment, KindSetting, KindDoc, KindAction, KindSaved:
		return k, true
	}
	k, ok := kindAliases[s]
	return k, ok
}

// Doc is one indexed thing. Prefix and Name name the resource whose
// visibility decides whether the doc may be returned, Ability is what the
// caller must hold on it.
type Doc struct {
	Ref       string
	Kind      Kind
	Title     string
	Body      string
	Tags      []string
	App       string
	Env       string
	Node      string
	Prefix    string
	Name      string
	Ability   string
	Confirm   bool
	Href      string
	ActionID  string
	UpdatedAt time.Time
}

// Validate rejects a doc that would bypass the authorization hook.
func (d Doc) Validate() error {
	switch {
	case d.Ref == "", d.Kind == "", d.Title == "":
		return errors.New("search: doc needs ref, kind and title")
	case d.Prefix == "" || !strings.HasSuffix(d.Prefix, ":"):
		return fmt.Errorf("search: doc %q has no resource prefix", d.Ref)
	case d.Name == "":
		return fmt.Errorf("search: doc %q has no resource name", d.Ref)
	case d.Ability == "":
		return fmt.Errorf("search: doc %q has no required ability", d.Ref)
	}
	return nil
}

// Provider produces every current doc of one kind.
type Provider interface {
	Kind() Kind
	Docs(ctx context.Context) ([]Doc, error)
}

// Authorizer answers whether the caller may see a resource with an ability.
// The API layer implements it with the same engine the path routes use.
type Authorizer interface {
	Allowed(prefix, name, ability string) bool
}

// Caller is who is searching.
type Caller struct {
	PrincipalID string
	Authz       Authorizer
}

// ActionInfo is what a client needs to run an action hit.
type ActionInfo struct {
	ID      string `json:"id"`
	Ability string `json:"ability"`
	Confirm bool   `json:"confirm"`
}

// Hit is one search result.
type Hit struct {
	Ref      string      `json:"ref"`
	Kind     Kind        `json:"kind"`
	Title    string      `json:"title"`
	Subtitle string      `json:"subtitle,omitempty"`
	AppName  string      `json:"app_name,omitempty"`
	Env      string      `json:"env,omitempty"`
	NodeName string      `json:"node_name,omitempty"`
	Tags     []string    `json:"tags,omitempty"`
	Href     string      `json:"href,omitempty"`
	Score    float64     `json:"score"`
	Action   *ActionInfo `json:"action,omitempty"`
}
