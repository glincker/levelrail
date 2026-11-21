package fielddesc

import "testing"

func TestBackupTargetVisibilityAndValidation(t *testing.T) {
	r, _ := Get(BackupTarget)
	if got := r.Validate(map[string]string{"provider": "local"}); len(got) != 2 {
		t.Fatalf("local needs name and path, got %v", got)
	}
	key := map[string]string{"provider": "sftp", "auth": "key", "name": "n", "endpoint": "h", "bucket": "b", "access_key_id": "u", "host_key": "k"}
	problems := r.Validate(key)
	if len(problems) != 1 || problems[0].Key != "private_key" {
		t.Fatalf("sftp key auth needs private_key only, got %v", problems)
	}
	if got := r.Validate(map[string]string{"provider": "nope", "name": "n"}); len(got) == 0 {
		t.Fatal("unknown provider must fail")
	}
}

func TestMaskedHidesSecrets(t *testing.T) {
	r, _ := Get(BackupTarget)
	m := r.Masked(map[string]string{"secret_access_key": "abc", "name": "x", "private_key": ""})
	if m["secret_access_key"] != SecretMask || m["name"] != "x" || m["private_key"] != "" {
		t.Fatalf("unexpected mask result %v", m)
	}
}

func TestLogDrainTargetRequiredOnlyForHTTP(t *testing.T) {
	r, _ := Get(LogDrain)
	if got := r.Validate(map[string]string{"type": "syslog"}); len(got) != 0 {
		t.Fatalf("syslog target is optional, got %v", got)
	}
	got := r.Validate(map[string]string{"type": "http"})
	if len(got) != 1 || got[0].Message != "target is required" {
		t.Fatalf("http needs a target, got %v", got)
	}
	if got := r.Validate(map[string]string{"type": "pigeon"}); len(got) == 0 {
		t.Fatal("unknown type must fail")
	}
}
