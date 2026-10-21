package forms

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Snippet returns a copy-paste HTML form generated from the form's schema.
func Snippet(endpoint string, f store.Form) string {
	return buildHTML(endpoint, f, true)
}

// SnippetMinimal returns bare markup: inputs with placeholders, no labels or styling hooks.
func SnippetMinimal(endpoint string, f store.Form) string {
	return buildHTML(endpoint, f, false)
}

// SnippetFetch returns an HTML form plus a script that posts it as JSON without leaving the page.
func SnippetFetch(endpoint string, f store.Form) string {
	s, _ := ParseSchema(f.Schema)
	msg := s.SuccessMessage
	if msg == "" {
		msg = "Thanks, your submission was received."
	}
	return strings.Replace(buildHTML(endpoint, f, true), "<form ", `<form id="lr-form" `, 1) + fmt.Sprintf(`
<p id="lr-status" role="status"></p>
<script>
document.getElementById("lr-form").addEventListener("submit", async function (e) {
  e.preventDefault();
  var data = Object.fromEntries(new FormData(e.target));
  var res = await fetch(%s, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Accept": "application/json" },
    body: JSON.stringify(data)
  });
  document.getElementById("lr-status").textContent = res.ok ? %s : "Something went wrong, please try again.";
  if (res.ok) e.target.reset();
});
</script>`, jsString(endpoint), jsString(msg))
}

func jsString(s string) string {
	return strings.ReplaceAll(strconv.Quote(s), "<", `<`)
}

func buildHTML(endpoint string, f store.Form, labels bool) string {
	s, _ := ParseSchema(f.Schema)
	hp := f.HoneypotField
	if hp == "" {
		hp = DefaultHoneypot
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<form action=\"%s\" method=\"POST\">\n", html.EscapeString(endpoint))
	fmt.Fprintf(&b, "  <p hidden><label>Leave empty <input name=\"%s\" tabindex=\"-1\" autocomplete=\"off\"></label></p>\n", html.EscapeString(hp))
	for _, fl := range s.EffectiveFields() {
		b.WriteString("  " + fieldHTML(fl, labels) + "\n")
	}
	b.WriteString("  <button type=\"submit\">Send</button>\n</form>")
	return b.String()
}

func fieldHTML(f Field, labels bool) string {
	name := html.EscapeString(f.Name)
	label := f.Label
	if label == "" {
		label = f.Name
	}
	var ctl string
	switch f.Type {
	case TypeHidden:
		return fmt.Sprintf(`<input type="hidden" name="%s" value="%s">`, name, html.EscapeString(f.Default))
	case TypeTextarea:
		ctl = "<textarea name=\"" + name + "\"" + attrs(f, labels) + "></textarea>"
	case TypeSelect:
		var o strings.Builder
		for _, opt := range f.Options {
			e := html.EscapeString(opt)
			o.WriteString(`<option value="` + e + `">` + e + `</option>`)
		}
		ctl = "<select name=\"" + name + "\"" + attrs(f, labels) + ">" + o.String() + "</select>"
	default:
		ctl = "<input type=\"" + f.Type + "\" name=\"" + name + "\"" + attrs(f, labels) + ">"
	}
	if !labels {
		return ctl
	}
	return "<label>" + html.EscapeString(label) + " " + ctl + "</label>"
}

func attrs(f Field, labels bool) string {
	var a string
	if f.Required {
		a += " required"
	}
	if f.Placeholder != "" {
		a += ` placeholder="` + html.EscapeString(f.Placeholder) + `"`
	} else if !labels && f.Type != TypeCheckbox && f.Type != TypeSelect {
		l := f.Label
		if l == "" {
			l = f.Name
		}
		a += ` placeholder="` + html.EscapeString(l) + `"`
	}
	if f.Type != TypeCheckbox && f.Type != TypeSelect {
		if f.MinLength > 0 {
			a += " minlength=\"" + strconv.Itoa(f.MinLength) + "\""
		}
		if f.MaxLength > 0 {
			a += " maxlength=\"" + strconv.Itoa(f.MaxLength) + "\""
		}
	}
	return a
}
