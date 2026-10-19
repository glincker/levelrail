package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

const panelBody = `contributes:
  panels:
    - id: deploys
      title: Recent deploys
      slot: app-detail
      source: { id: deploys, abilities: [read] }
      widgets:
        - { type: stat, title: Total, field: total, unit: deploys }
        - type: table
          title: Latest
          field: rows
          columns: [{ key: sha, label: Commit }, { key: status, label: Status }]
        - { type: markdown, field: note }
        - { type: button, action: refresh, label: Refresh, confirm: "Refresh now?" }
  commands:
    - { id: deploys, description: List deploys, panel: deploys }
    - { id: refresh, description: Refresh, panel: deploys, action: refresh }
`

func panelManifest(kind, abilities, body string) []byte {
	return []byte("api_version: 1\nname: example\nversion: 1.0.0\nkind: " + kind + "\npublisher: Example Org\n" +
		"image: registry.example.com/team/example@" + testDigest + "\nabilities: " + abilities + "\n" + body)
}

func TestPanelManifestValidation(t *testing.T) {
	bad := map[string]struct{ kind, abilities, body, want string }{
		"source needs panel_provider": {KindNotificationChannel, "[read]", panelBody, "needs kind panel_provider"},
		"write ability refused": {KindPanelProvider, "[read]", strings.Replace(panelBody, "abilities: [read] }", "abilities: [root] }", 1),
			"must be 'read'"},
		"source ability not requested": {KindPanelProvider, "[]", panelBody, "does not request"},
		"read:sensitive refused":       {KindPanelProvider, "[read, read:sensitive]", strings.Replace(panelBody, "abilities: [read] }", "abilities: [read:sensitive] }", 1), "schema"},
		"unknown widget":               {KindPanelProvider, "[read]", strings.Replace(panelBody, "type: stat", "type: iframe", 1), "schema"},
		"bad slot":                     {KindPanelProvider, "[read]", strings.Replace(panelBody, "app-detail", "everywhere", 1), "schema"},
		"bad field path":               {KindPanelProvider, "[read]", strings.Replace(panelBody, "field: total", "field: \"a..b\"", 1), "field"},
		"command names missing action": {KindPanelProvider, "[read]", strings.Replace(panelBody, "action: refresh }\n", "action: nuke }\n", 1), "no button"},
		"command names missing panel":  {KindPanelProvider, "[read]", strings.Replace(panelBody, "panel: deploys }", "panel: nope }", 1), "panel that has a source"},
		"table without columns":        {KindPanelProvider, "[read]", strings.Replace(panelBody, "columns: [{ key: sha, label: Commit }, { key: status, label: Status }]", "columns: []", 1), "columns"},
		"unknown widget field":         {KindPanelProvider, "[read]", strings.Replace(panelBody, "type: stat,", "type: stat, script: x,", 1), "parse manifest"},
	}
	for name, tc := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := ParseManifest(panelManifest(tc.kind, tc.abilities, tc.body))
			var inv *InvalidError
			if !errors.As(err, &inv) || !strings.Contains(inv.Msg, tc.want) {
				t.Fatalf("err = %v, want InvalidError containing %q", err, tc.want)
			}
		})
	}
	m, err := ParseManifest(panelManifest(KindPanelProvider, "[read]", panelBody))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.SlotPanels(SlotAppDetail); len(got) != 1 || got[0].Slot != SlotAppDetail {
		t.Fatalf("slot panels = %+v", got)
	}
	if len(m.SlotPanels(SlotDashboard)) != 0 {
		t.Fatal("panel leaked into another slot")
	}
}

func TestSanitizeMarkdown(t *testing.T) {
	cases := map[string]string{
		"plain **bold**":                          "plain **bold**",
		"<script>alert(1)</script>hi":             "alert(1)hi",
		"<img src=x onerror=alert(1)>t":           "t",
		"<<b>script>alert(1)<</b>/script>":        "script>alert(1)/script>",
		"[ok](https://example.com/a)":             "[ok](https://example.com/a)",
		"[bad](javascript:alert(1))":              "bad)",
		"[bad](data:text/html;base64,AAAA)":       "bad",
		"![pixel](https://tracker.example/p.png)": "pixel",
		"[ref]: javascript:alert(1)\ntext":        "\ntext",
		"a &lt;script&gt; b":                      "a script b",
		"line\x00with\u202econtrol":               "linewithcontrol",
		"[nested](https://a.example/<b>x</b>)":    "[nested](https://a.example/x)",
		"[bad](HTTP://EXAMPLE.COM)":               "bad",
		"[ok](http://a.example/x?y=1&z=2)":        "[ok](http://a.example/x?y=1&z=2)",
		"javascript:alert(1)":                     "alert(1)",
	}
	for in, want := range cases {
		if got := SanitizeMarkdown(in); got != want {
			t.Errorf("SanitizeMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SanitizeMarkdown(strings.Repeat("a", 100<<10)); len(got) > maxMarkdown {
		t.Fatalf("markdown not capped: %d", len(got))
	}
}

func TestBindPanelCapsAndCoerces(t *testing.T) {
	m, err := ParseManifest(panelManifest(KindPanelProvider, "[read]", panelBody))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := m.Panel("deploys")
	var rows strings.Builder
	rows.WriteString(`[`)
	for i := range maxViewRows + 50 {
		if i > 0 {
			rows.WriteString(",")
		}
		rows.WriteString(`{"sha":"` + strings.Repeat("x", 500) + `","status":{"nested":true}}`)
	}
	rows.WriteString(`]`)
	view, err := BindPanel(p, []byte(`{"total": 3, "rows": `+rows.String()+`, "note": "<b>hi</b> [x](javascript:1)"}`))
	if err != nil {
		t.Fatal(err)
	}
	if view.Widgets[0].Value != "3" || view.Widgets[0].Unit != "deploys" {
		t.Fatalf("stat = %+v", view.Widgets[0])
	}
	tbl := view.Widgets[1]
	if len(tbl.Rows) != maxViewRows || len(tbl.Rows[0][0]) != maxCellBytes || tbl.Rows[0][1] != "" {
		t.Fatalf("table rows=%d cell=%d status=%q", len(tbl.Rows), len(tbl.Rows[0][0]), tbl.Rows[0][1])
	}
	if view.Widgets[2].Text != "hi x" {
		t.Fatalf("markdown = %q", view.Widgets[2].Text)
	}
	if b := view.Widgets[3]; b.Action != "refresh" || b.Confirm != "Refresh now?" {
		t.Fatalf("button = %+v", b)
	}

	partial, err := BindPanel(p, []byte(`{"total": 1}`))
	if err != nil || partial.Widgets[0].Error != "" || partial.Widgets[1].Error == "" {
		t.Fatalf("a missing field must fail only its own widget: %+v, %v", partial, err)
	}
	for _, bad := range []string{`[]`, `"x"`, `null`, strings.Repeat(`{"a":`, 40) + `1` + strings.Repeat(`}`, 40)} {
		if _, err := BindPanel(p, []byte(bad)); !errors.Is(err, ErrBadData) {
			t.Errorf("BindPanel(%.20q) accepted", bad)
		}
	}
}

func panelHarness(t *testing.T, approve []string) *harness {
	t.Helper()
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.m.Install(ctx, h.request(panelManifest(KindPanelProvider, "[read]", panelBody)), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Approve(ctx, "example", approve, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	h.rpc.describe = &pluginpb.DescribeResponse{ContractVersion: 1, Name: "example", Version: "1.0.0", Kind: KindPanelProvider, RequestedAbilities: []string{AbilityRead}}
	h.rpc.panelResp = &pluginpb.PanelDataResponse{Json: `{"total": 2, "rows": [{"sha": "abc", "status": "ok"}], "note": "fine"}`}
	h.rpc.actionResp = &pluginpb.PanelActionResponse{Ok: true, Message: "done <b>now</b>"}
	h.m.Reconcile(ctx)
	if h.m.inst("example").connected() == nil {
		t.Fatal("plugin did not start")
	}
	return h
}

func TestPanelDataProxiesWithScopeAndAudit(t *testing.T) {
	h := panelHarness(t, []string{AbilityRead})
	view, err := h.m.PanelData(context.Background(), "example", "deploys", map[string]string{ScopeKeyAppID: "web"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Widgets[0].Value != "2" || view.Widgets[1].Rows[0][0] != "abc" {
		t.Fatalf("view = %+v", view)
	}
	req := h.rpc.panelReqs[0]
	if req.GetSourceId() != "deploys" || req.GetContext()[ScopeKeyAppID] != "web" {
		t.Fatalf("request = %v", req)
	}
	last := h.audit.events[len(h.audit.events)-1]
	if last.Action != AuditAbilityUsed || last.Detail["call"] != "panel_data" || last.Detail["abilities"] != "read" {
		t.Fatalf("audit = %+v", last)
	}
}

func TestPanelDataEnforcesApprovalKindScopeAndSize(t *testing.T) {
	h := panelHarness(t, []string{AbilityRead})
	ctx := context.Background()
	if _, err := h.m.PanelData(ctx, "example", "nope", nil); !errors.Is(err, ErrNoPanel) {
		t.Fatalf("unknown panel err = %v", err)
	}
	for _, scope := range []map[string]string{{"team": "x"}, {ScopeKeyAppID: "../../etc"}, {ScopeKeyAppID: ""}} {
		var inv *InvalidError
		if _, err := h.m.PanelData(ctx, "example", "deploys", scope); !errors.As(err, &inv) {
			t.Fatalf("scope %v err = %v", scope, err)
		}
	}
	h.rpc.panelResp = &pluginpb.PanelDataResponse{Json: `{"note":"` + strings.Repeat("a", h.m.cfg.MaxPanelBytes) + `"}`}
	if _, err := h.m.PanelData(ctx, "example", "deploys", nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize err = %v", err)
	}

	rec, _ := h.store.GetPlugin(ctx, "example")
	rec.ApprovedAbilities = []string{}
	if err := h.store.UpdatePlugin(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.PanelData(ctx, "example", "deploys", nil); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("unapproved err = %v", err)
	}
	if _, err := h.m.PanelAction(ctx, "example", "deploys", "refresh", nil); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("unapproved action err = %v", err)
	}
}

func TestPanelDataTimesOutAndAuditFailureRefuses(t *testing.T) {
	h := panelHarness(t, []string{AbilityRead})
	h.m.cfg.CallTimeout = 20 * time.Millisecond
	h.rpc.block = make(chan struct{})
	if _, err := h.m.PanelData(context.Background(), "example", "deploys", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow plugin err = %v", err)
	}
	h.rpc.block = nil
	h.audit.fail = errBoom
	if _, err := h.m.PanelData(context.Background(), "example", "deploys", nil); !errors.Is(err, errBoom) {
		t.Fatalf("audit failure err = %v", err)
	}
	if got := len(h.rpc.panelReqs); got != 0 {
		t.Fatalf("plugin was called %d times without an audit record", got)
	}
}

func TestPanelActionOnlyReachesDeclaredButtons(t *testing.T) {
	h := panelHarness(t, []string{AbilityRead})
	ctx := context.Background()
	res, err := h.m.PanelAction(ctx, "example", "deploys", "refresh", map[string]string{ScopeKeyAppID: "web"})
	if err != nil || !res.OK || res.Message != "done now" {
		t.Fatalf("res = %+v, %v", res, err)
	}
	if _, err := h.m.PanelAction(ctx, "example", "deploys", "nuke", nil); !errors.Is(err, ErrNoPanel) {
		t.Fatalf("undeclared action err = %v", err)
	}
	if len(h.rpc.actionReqs) != 1 {
		t.Fatalf("plugin saw %d action calls", len(h.rpc.actionReqs))
	}
}

func TestNotificationPluginHasNoPanelCalls(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead})
	if _, err := h.m.PanelData(context.Background(), "example", "deploys", nil); !errors.Is(err, ErrNotPanel) {
		t.Fatalf("err = %v", err)
	}
}
