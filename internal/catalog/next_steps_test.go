package catalog

import "testing"

func TestNextSteps_KnownTemplatesAndKeys(t *testing.T) {
	valid := map[string]bool{"open_admin": true, "create_login": true, "change_default_login": true, "point_domain": true}
	for id, steps := range nextStepsByID {
		if _, ok := TemplateByID(id); !ok {
			t.Errorf("next steps for %q: not in the catalog", id)
		}
		for _, s := range steps {
			if !valid[s.Key] {
				t.Errorf("%s: unknown step key %q", id, s.Key)
			}
			if s.Path != "" && s.Path[0] != '/' {
				t.Errorf("%s: path %q must start with /", id, s.Path)
			}
		}
	}
	if len(NextSteps("wordpress")) == 0 {
		t.Fatal("wordpress has no next steps")
	}
	if NextSteps("no-such-template") != nil {
		t.Fatal("unknown template must have no steps")
	}
}
