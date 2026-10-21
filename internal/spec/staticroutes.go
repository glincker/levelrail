package spec

import (
	"fmt"
	"sort"
)

// Route is one entry under a static service's routes:. Status defaults to 301.
type Route struct {
	From   string            `yaml:"from"`
	To     string            `yaml:"to"`
	Status int               `yaml:"status,omitempty"`
	Force  bool              `yaml:"force,omitempty"`
	Query  map[string]string `yaml:"query,omitempty"`
}

// HeaderRule is one entry under a static service's headers:.
type HeaderRule struct {
	Path   string            `yaml:"path"`
	Values map[string]string `yaml:"values,omitempty"`
	Remove []string          `yaml:"remove,omitempty"`
}

// StaticOptions is a static service's static: block.
type StaticOptions struct {
	SPA           bool   `yaml:"spa,omitempty"`
	CleanURLs     bool   `yaml:"cleanUrls,omitempty"`
	TrailingSlash string `yaml:"trailingSlash,omitempty"`
	NotFound      string `yaml:"notFound,omitempty"`
}

// StaticInput is what a deploy hands to the static routes compiler.
type StaticInput struct {
	Redirects string
	Headers   string
	// Routes and HeaderBlocks come from app.yaml and rank before the files.
	Routes       []StaticRule
	HeaderBlocks []StaticHeaderBlock
	Options      StaticServeOptions
}

// StaticRoutingInput converts the routing fields to the form the deploy
// compiles; the _redirects and _headers file texts are filled in by the caller.
func (s Service) StaticRoutingInput() StaticInput {
	in := StaticInput{}
	for _, r := range s.Routes {
		rule := StaticRule{From: r.From, To: r.To, Status: r.Status, Force: r.Force}
		keys := make([]string, 0, len(r.Query))
		for k := range r.Query {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			rule.Query = append(rule.Query, StaticQueryMatch{Key: k, Value: r.Query[k]})
		}
		in.Routes = append(in.Routes, rule)
	}
	for _, h := range s.Headers {
		b := StaticHeaderBlock{Path: h.Path, Remove: h.Remove}
		names := make([]string, 0, len(h.Values))
		for n := range h.Values {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			b.Set = append(b.Set, StaticHeaderKV{Name: n, Value: h.Values[n]})
		}
		in.HeaderBlocks = append(in.HeaderBlocks, b)
	}
	if s.Static != nil {
		in.Options = StaticServeOptions{SPA: s.Static.SPA, CleanURLs: s.Static.CleanURLs, TrailingSlash: s.Static.TrailingSlash, NotFound: s.Static.NotFound}
	}
	return in
}

func (s Service) validateStaticRouting(name string) error {
	if len(s.Routes) == 0 && len(s.Headers) == 0 && s.Static == nil {
		return nil
	}
	if s.Build.Type != BuildStatic {
		return fmt.Errorf("spec: service %q: routes, headers and static are only meaningful for build.type: static", name)
	}
	in := s.StaticRoutingInput()
	l := DefaultStaticLimits()
	if _, _, err := CompileStaticEntries(in.Routes, in.HeaderBlocks, &in.Options, l); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	if len(in.Routes) > l.MaxRules {
		return fmt.Errorf("spec: service %q: %d routing rules is over the cap of %d (%s)", name, len(in.Routes), l.MaxRules, EnvStaticMaxRules)
	}
	if len(in.HeaderBlocks) > l.MaxHeaderBlocks {
		return fmt.Errorf("spec: service %q: %d header blocks is over the cap of %d (%s)", name, len(in.HeaderBlocks), l.MaxHeaderBlocks, EnvStaticMaxHeaderBlocks)
	}
	return nil
}
