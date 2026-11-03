package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/hostprovider"
)

// consoleRoots are stable provider console front doors only, never deep links.
var consoleRoots = map[hostprovider.ID]string{
	hostprovider.Oracle:       "https://cloud.oracle.com/",
	hostprovider.AWS:          "https://console.aws.amazon.com/",
	hostprovider.GCP:          "https://console.cloud.google.com/",
	hostprovider.Azure:        "https://portal.azure.com/",
	hostprovider.DigitalOcean: "https://cloud.digitalocean.com/",
	hostprovider.Hetzner:      "https://console.hetzner.cloud/",
}

const genericCloudLine = "Provider not detected. Allow inbound TCP 80 and 443 (keep 22 for SSH) in your provider's firewall or security group."

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
func buildCloudReport(g hostprovider.Guidance, found bool) cloudReport {
	r := cloudReport{Provider: "unknown"}
	ports := []hostprovider.PortRule{{Port: 80, Protocol: "tcp", Source: "0.0.0.0/0 and ::/0"}, {Port: 443, Protocol: "tcp", Source: "0.0.0.0/0 and ::/0"}}
	if found {
		r.Provider, r.Detected = string(g.ID), true
		r.ConsoleURL = consoleRoots[g.ID]
		r.DocsURL, r.Steps, r.Note = g.DocsURL, g.Steps, g.Note
		if len(g.Ports) > 0 {
			ports = g.Ports
		}
	}
	for _, p := range ports {
		name := "levelrail-http"
		if p.Port == 443 {
			name = "levelrail-https"
		}
		r.Rules = append(r.Rules, cloudRule{Name: name, Port: p.Port, Protocol: p.Protocol, Source: p.Source})
	}
	return r
}

func printCloudReport(w io.Writer, r cloudReport, g hostprovider.Guidance) {
	if !r.Detected {
		_, _ = fmt.Fprintln(w, genericCloudLine)
		return
	}
	_, _ = fmt.Fprintf(w, "Provider: %s (detected from %s)\nCreate these inbound rules:\n", g.Name, g.Source)
	for _, x := range r.Rules {
		_, _ = fmt.Fprintf(w, "  %-16s %s %d from %s\n", x.Name, strings.ToUpper(x.Protocol), x.Port, x.Source)
	}
	if r.ConsoleURL != "" {
		_, _ = fmt.Fprintf(w, "Console: %s\n", r.ConsoleURL)
	}
	for i, s := range r.Steps {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, s)
	}
	if r.Note != "" {
		_, _ = fmt.Fprintf(w, "Note: %s\n", r.Note)
	}
	if r.DocsURL != "" {
		_, _ = fmt.Fprintf(w, "Provider docs: %s\n", r.DocsURL)
	}
}

// runDoctorCloud prints the provider firewall rules for this host. It is
// offline apart from the link-local metadata probe and needs no sign-in.
func runDoctorCloud(prog string, args []string, stdout, stderr io.Writer, deps hostprovider.Deps) int {
	fs := flag.NewFlagSet("doctor cloud", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the rules as JSON to stdout and nothing else")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s doctor cloud [--json]\n  %s doctor --cloud [--json]\n\nPrints the inbound firewall rules (names, ports 80 and 443) and the console\nlink for the cloud provider this server runs on, from its metadata service.\nAn unknown provider gets the generic ports line.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	g, found := hostprovider.Detect(context.Background(), deps)
	r := buildCloudReport(g, found)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitUsage
		}
		return exitOK
	}
	printCloudReport(stdout, r, g)
	return exitOK
}
