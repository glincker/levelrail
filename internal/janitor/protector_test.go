package janitor

import "testing"

func TestSetVolumeProtectorAddsToDefaultRule(t *testing.T) {
	t.Cleanup(func() { SetVolumeProtector(nil) })
	SetVolumeProtector(func(name string) (bool, string) { return name == "owned-by-db", "owned by a database" })
	if prot, _ := volumeProtected("owned-by-db"); !prot {
		t.Error("a volume the installed lock owns must be protected")
	}
	if prot, _ := volumeProtected("db-main-data"); !prot {
		t.Error("the db- rule must still apply with a lock installed")
	}
	if prot, _ := volumeProtected("scratch"); prot {
		t.Error("an ordinary volume must not be protected")
	}
	SetVolumeProtector(nil)
	if prot, _ := volumeProtected("owned-by-db"); prot {
		t.Error("clearing the lock must restore the default rule only")
	}
}
