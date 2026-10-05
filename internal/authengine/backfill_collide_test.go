package authengine

import (
	"strings"
	"testing"
)

func TestCheckEmailCollisionsListsGroups(t *testing.T) {
	users := []legacyUser{
		{id: "u1", email: "A@x.io"}, {id: "u2", email: "b@x.io"}, {id: "u3", email: "a@x.io"},
	}
	err := checkEmailCollisions(users)
	if err == nil {
		t.Fatal("want collision error")
	}
	for _, want := range []string{"1 user emails collide", "u1 (A@x.io)", "u3 (a@x.io)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "u2") {
		t.Errorf("non-colliding user listed: %v", err)
	}
	if checkEmailCollisions(users[:2]) != nil {
		t.Error("unique emails must pass")
	}
}
