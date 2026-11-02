package configtranslate

import (
	"strings"
	"testing"
)

func TestVercelCutoverChecklistEmitted(t *testing.T) {
	in, err := Translate(loadCase(t, "vercel-next")).EmitInput(EmitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(in.Notes, "\n"), "Cutover checklist") {
		t.Fatal("vercel source must add the cutover checklist")
	}
	in, err = Translate(loadCase(t, "heroku-minimal")).EmitInput(EmitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(in.Notes, "\n"), "Cutover checklist") {
		t.Fatal("non-vercel source must not get the checklist")
	}
}

func TestNextStandaloneHint(t *testing.T) {
	has := func(files map[string][]byte) bool {
		for _, f := range nextStandaloneFinding(files, "nextjs", false) {
			if f.Code == "vercel.next.standalone" {
				return true
			}
		}
		return false
	}
	if !has(map[string][]byte{"next.config.js": []byte("module.exports = {}")}) {
		t.Error("config without standalone must give the hint")
	}
	if has(map[string][]byte{"next.config.mjs": []byte(`export default { output: "standalone" }`)}) {
		t.Error("config with standalone must stay quiet")
	}
	if !has(map[string][]byte{"next.config.js": []byte("// standalone later\nmodule.exports = {}")}) {
		t.Error("a comment mentioning standalone must not silence the hint")
	}
	if has(map[string][]byte{"next.config.ts": []byte("const c = { output: 'standalone' }\nexport default c")}) {
		t.Error("single-quoted standalone must stay quiet")
	}
	if len(nextStandaloneFinding(nil, "vite", false)) != 0 {
		t.Error("non-Next framework must stay quiet")
	}
}
