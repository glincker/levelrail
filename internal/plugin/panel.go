package plugin

import (
	"regexp"
	"sort"
)

// KindPanelProvider is a plugin that only contributes panels and commands.
const KindPanelProvider = "panel_provider"

// Placement slots a panel can be rendered in.
const (
	SlotPlugins   = "plugins"
	SlotAppDetail = "app-detail"
	SlotSettings  = "settings"
	SlotDashboard = "dashboard"
)

// Widget types. The set is closed: the dashboard renders only these.
const (
	WidgetStat     = "stat"
	WidgetTable    = "table"
	WidgetList     = "list"
	WidgetChart    = "chart"
	WidgetMarkdown = "markdown"
	WidgetStatus   = "status"
	WidgetButton   = "button"
)

const (
	maxPanels   = 8
	maxWidgets  = 12
	maxColumns  = 8
	maxCommands = 16
)

var (
	idPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	fieldPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}(\.[A-Za-z0-9_]{1,40}){0,5}$`)
	keyPattern   = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)
)

// PanelSource is the plugin call that feeds a panel. Abilities are what the
// host hands the call, and must be a subset of what the operator approved.
type PanelSource struct {
	ID        string   `yaml:"id" json:"id"`
	Abilities []string `yaml:"abilities" json:"abilities"`
}

// Column maps a key in each table row object to a header.
type Column struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label" json:"label"`
}

// Widget is one element of a panel. Field is a dotted path into the source's
// JSON document.
type Widget struct {
	Type     string   `yaml:"type" json:"type"`
	Title    string   `yaml:"title" json:"title"`
	Field    string   `yaml:"field" json:"field,omitempty"`
	Unit     string   `yaml:"unit" json:"unit,omitempty"`
	Columns  []Column `yaml:"columns" json:"columns,omitempty"`
	LabelKey string   `yaml:"label_key" json:"label_key,omitempty"`
	ValueKey string   `yaml:"value_key" json:"value_key,omitempty"`
	Action   string   `yaml:"action" json:"action,omitempty"`
	Label    string   `yaml:"label" json:"label,omitempty"`
	Confirm  string   `yaml:"confirm" json:"confirm,omitempty"`
}

// Command is a CLI subcommand (`plugins run <plugin> <id>`). With Action it
// presses that panel button, otherwise it prints the panel's data.
type Command struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
	Panel       string `yaml:"panel" json:"panel"`
	Action      string `yaml:"action" json:"action,omitempty"`
}

func (m *Manifest) validatePanels() error {
	if len(m.Contributes.Panels) > maxPanels {
		return invalid("at most %d panels", maxPanels)
	}
	if len(m.Contributes.Commands) > maxCommands {
		return invalid("at most %d commands", maxCommands)
	}
	byID := map[string]*Panel{}
	for i := range m.Contributes.Panels {
		p := &m.Contributes.Panels[i]
		if !idPattern.MatchString(p.ID) {
			return invalid("panel id %q must be lowercase letters, digits or dashes", p.ID)
		}
		if byID[p.ID] != nil {
			return invalid("panel id %q is used twice", p.ID)
		}
		byID[p.ID] = p
		if err := m.validatePanel(p); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, c := range m.Contributes.Commands {
		if !idPattern.MatchString(c.ID) || seen[c.ID] {
			return invalid("command id %q must be unique lowercase letters, digits or dashes", c.ID)
		}
		seen[c.ID] = true
		if len(c.Description) > 200 {
			return invalid("command %q description is 200 characters at most", c.ID)
		}
		p := byID[c.Panel]
		if p == nil || p.Source == nil {
			return invalid("command %q must name a panel that has a source", c.ID)
		}
		if c.Action != "" && !p.hasAction(c.Action) {
			return invalid("command %q names action %q, which no button in panel %q declares", c.ID, c.Action, c.Panel)
		}
	}
	return nil
}

func (m *Manifest) validatePanel(p *Panel) error {
	if p.Slot == "" {
		p.Slot = SlotPlugins
	}
	switch p.Slot {
	case SlotPlugins, SlotAppDetail, SlotSettings, SlotDashboard:
	default:
		return invalid("panel %q slot %q is not one of plugins, app-detail, settings, dashboard", p.ID, p.Slot)
	}
	if p.Source == nil {
		if len(p.Widgets) > 0 {
			return invalid("panel %q has widgets but no source to feed them", p.ID)
		}
		return nil
	}
	if len(p.Rows) > 0 {
		return invalid("panel %q cannot mix static rows with a source", p.ID)
	}
	if m.Kind != KindPanelProvider {
		return invalid("panel %q has a source, which needs kind %s", p.ID, KindPanelProvider)
	}
	if !idPattern.MatchString(p.Source.ID) {
		return invalid("panel %q source id must be lowercase letters, digits or dashes", p.ID)
	}
	if err := m.validateSourceAbilities(p); err != nil {
		return err
	}
	if len(p.Widgets) == 0 || len(p.Widgets) > maxWidgets {
		return invalid("panel %q needs 1 to %d widgets", p.ID, maxWidgets)
	}
	for i, w := range p.Widgets {
		if msg := w.problem(); msg != "" {
			return invalid("panel %q widget %d: %s", p.ID, i+1, msg)
		}
	}
	return nil
}

func (m *Manifest) validateSourceAbilities(p *Panel) error {
	if len(p.Source.Abilities) == 0 {
		return invalid("panel %q source must list the abilities it uses (read)", p.ID)
	}
	abilities, err := normalizeAbilities(p.Source.Abilities)
	if err != nil {
		return err
	}
	for _, a := range abilities {
		if a != AbilityRead {
			return invalid("panel %q source may only use the read ability, %q is not allowed on panels", p.ID, a)
		}
		if !contains(m.Abilities, a) {
			return invalid("panel %q source uses %q, which the plugin does not request", p.ID, a)
		}
	}
	p.Source.Abilities = abilities
	return nil
}

// problem returns why the widget is invalid, or "" when it is fine.
func (w Widget) problem() string {
	if len(w.Title) > 80 || len(w.Unit) > 16 || len(w.Label) > 60 || len(w.Confirm) > 200 {
		return "title, unit, label or confirm is too long"
	}
	switch w.Type {
	case WidgetStat, WidgetList, WidgetMarkdown, WidgetStatus:
		return w.fieldProblem()
	case WidgetTable:
		if msg := w.fieldProblem(); msg != "" {
			return msg
		}
		if len(w.Columns) == 0 || len(w.Columns) > maxColumns {
			return "a table needs 1 to 8 columns"
		}
		for _, c := range w.Columns {
			if !keyPattern.MatchString(c.Key) || c.Label == "" || len(c.Label) > 60 {
				return "every table column needs a simple key and a label of 60 characters at most"
			}
		}
	case WidgetChart:
		if msg := w.fieldProblem(); msg != "" {
			return msg
		}
		if !keyPattern.MatchString(w.LabelKey) || !keyPattern.MatchString(w.ValueKey) {
			return "a chart needs label_key and value_key"
		}
	case WidgetButton:
		if !idPattern.MatchString(w.Action) || w.Label == "" {
			return "a button needs an action id and a label"
		}
	default:
		return "type is not one of stat, table, list, chart, markdown, status, button"
	}
	return ""
}

func (w Widget) fieldProblem() string {
	if !fieldPattern.MatchString(w.Field) {
		return "field must be a dotted path of letters, digits and underscores"
	}
	return ""
}

func (p *Panel) hasAction(id string) bool {
	for _, w := range p.Widgets {
		if w.Type == WidgetButton && w.Action == id {
			return true
		}
	}
	return false
}

// Panel returns the named panel of the manifest.
func (m Manifest) Panel(id string) (Panel, bool) {
	for _, p := range m.Contributes.Panels {
		if p.ID == id {
			return p, true
		}
	}
	return Panel{}, false
}

// SlotPanels returns data-backed panels placed in slot, sorted by id.
func (m Manifest) SlotPanels(slot string) []Panel {
	out := []Panel{}
	for _, p := range m.Contributes.Panels {
		if p.Source != nil && p.Slot == slot {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
