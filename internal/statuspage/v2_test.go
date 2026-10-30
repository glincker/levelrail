package statuspage

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func v2View() View {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	up := 99.5
	return View{Title: "Acme <b>", Status: Degraded, StatusText: "Degraded performance", GeneratedAt: now,
		Accent: "#ff6600", LogoURL: "https://cdn.example.com/l.png", About: "About <script>x</script>", Website: "https://example.com",
		NoIndex: true, PoweredBy: true,
		Components: []ComponentView{
			{ID: "sc_1", Name: "Web <script>alert(1)</script>", Description: "Public site", Group: "Core", Status: Operational, Uptime90: &up,
				Days: []DayView{{Date: "2026-10-01", Status: Degraded, Level: 2}}},
			{ID: "sc_2", Name: "API", Group: "Core", Status: NoData},
			{ID: "sc_3", Name: "Docs", Status: Operational, Uptime90: &up},
		},
		Incidents: []IncidentView{{ID: "si_1", Title: "Bad <script>alert(2)</script>", Status: StatusInvestigating, Impact: ImpactMinor, StartsAt: now,
			Updates: []UpdateView{{Status: StatusInvestigating, Body: "we <script>alert(3)</script> look", At: now}}}},
		Maintenance: []IncidentView{{ID: "w_1", Title: "Window", Status: StatusScheduled, Impact: Maintenance, StartsAt: now.Add(time.Hour)}},
	}
}

func TestRenderHTML_GroupsBrandingAndNoData(t *testing.T) {
	out, err := RenderHTML(v2View())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`<meta name="robots" content="noindex">`, `<details class="group" open>`, `<span>Core</span>`,
		`--accent:#ff6600`, `src="https://cdn.example.com/l.png"`, `Public site`, `No data`, `bar degraded lv2`,
		`/public/status/incidents/si_1`, `rel="noopener noreferrer">Levelrail`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(s, "<script>") {
		t.Error("page contains a raw script tag")
	}
	v := v2View()
	v.NoIndex, v.Accent, v.PoweredBy = false, "red;}body{display:none", false
	out, _ = RenderHTML(v)
	s = string(out)
	if strings.Contains(s, "noindex") || strings.Contains(s, "display:none") || strings.Contains(s, "Powered by") {
		t.Error("noindex, invalid accent and credit must be omitted when unset")
	}
}

func TestHistoryAndIncidentPages(t *testing.T) {
	body, err := RenderHistory(v2View(), "maintenance", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Window") || strings.Contains(string(body), "Bad ") {
		t.Error("maintenance tab must list only maintenance")
	}
	page, ok, err := RenderIncident(v2View(), "si_1")
	if err != nil || !ok {
		t.Fatalf("incident page: ok=%v err=%v", ok, err)
	}
	if strings.Contains(string(page), "<script>") || !strings.Contains(string(page), "&lt;script&gt;") {
		t.Error("incident text must be HTML-escaped")
	}
	if _, ok, _ := RenderIncident(v2View(), "nope"); ok {
		t.Error("unknown incident must not render")
	}
}

func TestRenderAtom_WellFormed(t *testing.T) {
	out, err := RenderAtom(v2View(), "https://status.example.com")
	if err != nil {
		t.Fatal(err)
	}
	var f atomFeed
	if err := xml.Unmarshal(out, &f); err != nil {
		t.Fatalf("not well-formed: %v", err)
	}
	if f.XMLName.Space != "http://www.w3.org/2005/Atom" || len(f.Entries) != 2 || f.Entries[0].ID != "https://status.example.com/public/status/incidents/si_1" {
		t.Errorf("unexpected feed: %+v", f)
	}
	if !strings.Contains(f.Entries[0].Title, "<script>") {
		t.Error("title must round-trip as text, escaped on the wire")
	}
}

func TestRenderBadge_Escapes(t *testing.T) {
	svg := string(RenderBadge(`x"><script>alert(1)</script>`, Outage))
	if strings.Contains(svg, "<script>") || strings.Contains(svg, `x">`) {
		t.Errorf("badge leaked markup: %s", svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
		t.Errorf("badge is not well-formed: %v", err)
	}
	if !strings.Contains(string(RenderBadge("status", Operational)), "operational") {
		t.Error("badge must carry the state label")
	}
}

func TestDayLevelAndStatus(t *testing.T) {
	t.Setenv("APP_STATUS_DEGRADED_WEIGHT", "0.5")
	t.Setenv("APP_STATUS_DAY_OUTAGE_SHARE", "0.05")
	tn := TuningFromEnv()
	one, lvl := dayStatus(store.StatusDaily{OK: 1439, Down: 1}, tn)
	if one != Degraded || lvl != 1 {
		t.Errorf("one failed sample = %s/%d, want degraded/1", one, lvl)
	}
	if st, lvl := dayStatus(store.StatusDaily{OK: 90, Down: 10}, tn); st != Outage || lvl != 4 {
		t.Errorf("10%% down = %s/%d, want outage/4", st, lvl)
	}
	if st, _ := dayStatus(store.StatusDaily{}, tn); st != NoData {
		t.Errorf("empty day = %s, want nodata", st)
	}
	if dayLevel(0.025, tn) != 3 {
		t.Errorf("half the outage share must give level 3, got %d", dayLevel(0.025, tn))
	}
}

func TestUptimeWeighting(t *testing.T) {
	tn := Tuning{DegradedWeight: 0.5}
	u, _ := uptimeFraction(80, 10, 10, tn)
	if u != 0.85 {
		t.Errorf("uptime = %v, want 0.85", u)
	}
	tn.DegradedWeight = 0
	if u, _ := uptimeFraction(80, 10, 10, tn); u != 0.9 {
		t.Errorf("weight 0 uptime = %v, want 0.9", u)
	}
	if _, has := uptimeFraction(0, 0, 0, tn); has {
		t.Error("no samples must report no data")
	}
}
