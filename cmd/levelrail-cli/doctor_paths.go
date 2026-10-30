package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
)

const (
	envReleasesDir = "LEVELRAIL_RELEASES_DIR"
	envInstallDir  = "LEVELRAIL_INSTALL_DIR"
	envHTTPAddr    = "APP_HTTP_ADDR"

	defaultInstallDir = "/usr/local/bin"
	defaultDataDir    = "/var/lib/levelrail-data"
	systemdUnitPath   = "/etc/systemd/system/levelrail.service"
	systemdDropinPath = "/etc/systemd/system/levelrail.service.d/10-install.conf"
)

type doctorPath struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type doctorPathsResource struct {
	Paths    []doctorPath `json:"paths"`
	HTTPAddr string       `json:"http_addr,omitempty"`
}

// buildDoctorPaths is offline on purpose: an operator on the host needs the
// layout before any API URL or token works.
func buildDoctorPaths(lookupEnv func(string) (string, bool), exists func(string) bool) doctorPathsResource {
	get := func(key, def string) string {
		if v, ok := lookupEnv(key); ok && v != "" {
			return v
		}
		return def
	}
	installDir := get(envInstallDir, defaultInstallDir)
	releases := get(envReleasesDir, filepath.Join(installDir, "levelrail.releases"))
	entries := []doctorPath{
		{Name: "control plane binary", Path: filepath.Join(installDir, "levelrail")},
		{Name: "releases dir (rollback binaries)", Path: releases},
		{Name: "data dir", Path: get(envDataDir, defaultDataDir)},
		{Name: "systemd unit", Path: systemdUnitPath},
		{Name: "systemd drop-in (addresses)", Path: systemdDropinPath},
	}
	if self, err := os.Executable(); err == nil {
		entries = append([]doctorPath{{Name: "this CLI", Path: self}}, entries...)
	}
	for i := range entries {
		entries[i].Exists = exists(entries[i].Path)
	}
	return doctorPathsResource{Paths: entries, HTTPAddr: get(envHTTPAddr, "")}
}

func runDoctorPaths(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "doctor paths", "print the paths as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s doctor paths [flags]\n\nPrints where the binary, releases dir, data dir and systemd unit live on this host.\nIt never contacts a control plane.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	_, _, _, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	res := buildDoctorPaths(lookupEnv, func(p string) bool { _, err := os.Stat(p); return err == nil })
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "WHAT\tPATH\tFOUND")
		for _, p := range res.Paths {
			found := "no"
			if p.Exists {
				found = "yes"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.Path, found)
		}
		_ = tw.Flush()
		if res.HTTPAddr != "" {
			_, _ = fmt.Fprintf(stdout, "API address (%s): %s\n", envHTTPAddr, res.HTTPAddr)
		}
	}); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	return exitOK
}
