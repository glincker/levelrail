package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/hostprovider"
)

type cloudRule struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}

type cloudReport struct {
	Provider   string      `json:"provider"`
	Detected   bool        `json:"detected"`
	Rules      []cloudRule `json:"rules"`
	ConsoleURL string      `json:"console_url,omitempty"`
	DocsURL    string      `json:"docs_url,omitempty"`
	Steps      []string    `json:"steps,omitempty"`
	Note       string      `json:"note,omitempty"`
}

// buildCloudReport turns a detection result into the rules to create. An
// undetected provider still gets the two web ports so the output is never empty.
func buildCloudReport(g hostprovider.Guidance, found bool, cat stringCatalog) cloudReport {
	r := cloudReport{Provider: "unknown"}
	ports := []hostprovider.PortRule{{Port: 80, Protocol: "tcp", Source: "0.0.0.0/0 and ::/0"}, {Port: 443, Protocol: "tcp", Source: "0.0.0.0/0 and ::/0"}}
	if found {
		r.Provider, r.Detected = string(g.ID), true
		r.ConsoleURL = cat["console_url."+string(g.ID)]
		r.DocsURL, r.Steps, r.Note = g.DocsURL, g.Steps, g.Note
		if len(g.Ports) > 0 {
			ports = g.Ports
		}
	}
	binary := brandBinaryName()
	for _, p := range ports {
		name := cat.render("rule_name_http", "binary", binary)
		if p.Port == 443 {
			name = cat.render("rule_name_https", "binary", binary)
		}
		r.Rules = append(r.Rules, cloudRule{Name: name, Port: p.Port, Protocol: p.Protocol, Source: p.Source})
	}
	return r
}

func printCloudReport(w io.Writer, r cloudReport, g hostprovider.Guidance, cat stringCatalog) {
	if !r.Detected {
		_, _ = fmt.Fprintln(w, cat.render("generic_line"))
		return
	}
	_, _ = fmt.Fprint(w, cat.render("provider_line", "name", g.Name, "source", g.Source))
	for _, x := range r.Rules {
		_, _ = fmt.Fprint(w, cat.render("rule_line", "rule", fmt.Sprintf("%-16s", x.Name), "protocol", strings.ToUpper(x.Protocol), "port", strconv.Itoa(x.Port), "source", x.Source))
	}
	if r.ConsoleURL != "" {
		_, _ = fmt.Fprint(w, cat.render("console_line", "url", r.ConsoleURL))
	}
	for i, s := range r.Steps {
		_, _ = fmt.Fprint(w, cat.render("step_line", "index", strconv.Itoa(i+1), "step", s))
	}
	if r.Note != "" {
		_, _ = fmt.Fprint(w, cat.render("note_line", "note", r.Note))
	}
	if r.DocsURL != "" {
		_, _ = fmt.Fprint(w, cat.render("docs_line", "url", r.DocsURL))
	}
}

// runDoctorCloud prints the provider firewall rules for this host. It is
// offline apart from the link-local metadata probe and needs no sign-in.
func runDoctorCloud(prog string, args []string, stdout, stderr io.Writer, deps hostprovider.Deps) int {
	cat, cerr := loadDoctorCloudStrings()
	if cat == nil {
		_, _ = fmt.Fprintln(stderr, cerr)
		return exitUsage
	}
	if cerr != nil {
		_, _ = fmt.Fprintln(stderr, cerr)
	}
	fs := flag.NewFlagSet("doctor cloud", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, cat.render("flag_json"))
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, cat.render("usage", "prog", prog))
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	g, found := hostprovider.Detect(context.Background(), deps)
	r := buildCloudReport(g, found, cat)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		return exitOK
	}
	printCloudReport(stdout, r, g, cat)
	return exitOK
}
