package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// anchorFetchTimeout bounds reading a remote export; override with APP_AUDIT_ANCHOR_FETCH_TIMEOUT.
const (
	envAnchorFetchTimeout = "APP_AUDIT_ANCHOR_FETCH_TIMEOUT"
	defaultAnchorFetch    = 30 * time.Second
	anchorExportMaxBytes  = 64 << 20
)

type auditVerifyAnchorResult struct {
	Chain  apiclient.AuditChainReport  `json:"chain"`
	Anchor apiclient.AuditAnchorReport `json:"anchor"`
}

func runAuditVerifyAnchor(client *apiclient.Client, src string, chain apiclient.AuditChainReport, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	raw, err := readAnchorExport(src)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	anchors, err := parseAnchorExport(raw)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	rep, err := client.CheckAuditAnchors(context.Background(), anchors)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check audit anchors: %w", err))
	}
	res := auditVerifyAnchorResult{Chain: chain, Anchor: rep}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if chain.OK {
			_, _ = fmt.Fprintf(stdout, "audit log intact: %d entries checked, %d checkpoints\n", chain.Checked, chain.Checkpoints)
		} else {
			_, _ = fmt.Fprintf(stdout, "audit log FAILED verification: %d problems\n", len(chain.Problems))
			for _, p := range chain.Problems {
				_, _ = fmt.Fprintf(stdout, "  entry %d: %s\n", p.Seq, p.Reason)
			}
		}
		if rep.OK {
			_, _ = fmt.Fprintf(stdout, "anchors match: %d of %d anchored heads found in the live chain (%d compacted by retention, %d signatures unverified), live head %d\n", rep.Matched, rep.Checked, rep.Compacted, rep.Unverified, rep.LiveHead)
			return
		}
		_, _ = fmt.Fprintf(stdout, "anchor check FAILED: %d problems\n", len(rep.Problems))
		for _, p := range rep.Problems {
			_, _ = fmt.Fprintf(stdout, "  anchor %d: %s\n", p.Seq, p.Reason)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !chain.OK || !rep.OK {
		return exitCheckFailed
	}
	return exitOK
}

func readAnchorExport(src string) ([]byte, error) {
	if !strings.HasPrefix(src, "https://") {
		b, err := os.ReadFile(src) //nolint:gosec // the operator names their own export file
		if err != nil {
			return nil, fmt.Errorf("read anchor export: %w", err)
		}
		return b, nil
	}
	timeout := defaultAnchorFetch
	if d, err := time.ParseDuration(os.Getenv(envAnchorFetchTimeout)); err == nil && d > 0 {
		timeout = d
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, fmt.Errorf("anchor export url: %w", err)
	}
	resp, err := netguard.NewClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch anchor export: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("fetch anchor export: server answered %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, anchorExportMaxBytes))
	if err != nil {
		return nil, fmt.Errorf("fetch anchor export: %w", err)
	}
	return b, nil
}

// parseAnchorExport accepts the JSON-lines file the sink appends to, or a JSON array.
func parseAnchorExport(raw []byte) ([]apiclient.AuditAnchor, error) {
	trimmed := bytes.TrimSpace(raw)
	var out []apiclient.AuditAnchor
	if bytes.HasPrefix(trimmed, []byte("[")) {
		if err := json.Unmarshal(trimmed, &out); err != nil {
			return nil, fmt.Errorf("anchor export is not valid JSON: %w", err)
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(trimmed))
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for n := 1; sc.Scan(); n++ {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var a apiclient.AuditAnchor
			if err := json.Unmarshal([]byte(line), &a); err != nil {
				return nil, fmt.Errorf("anchor export line %d is not valid JSON: %w", n, err)
			}
			out = append(out, a)
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("read anchor export: %w", err)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("anchor export holds no anchors")
	}
	return out, nil
}

func runAuditAnchorStatus(client *apiclient.Client, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	st, err := client.AuditAnchorStatus(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read anchor status: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() {
		if !st.Enabled {
			_, _ = fmt.Fprintln(stdout, "anchor webhook export is off (set audit.anchor_webhook_url to turn it on)")
			return
		}
		_, _ = fmt.Fprintf(stdout, "last exported sequence: %d (%d exported, %d failed, %d deferred, %d queued)\n", st.LastExportedSeq, st.Exported, st.Failed, st.Dropped, st.Queued)
		if st.LastError != "" {
			_, _ = fmt.Fprintf(stdout, "last error at %s: %s\n", st.LastErrorAt, st.LastError)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
