package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const sshCLIKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-cli-key\n-----END OPENSSH PRIVATE KEY-----\n"

type sshAPI struct {
	calls []string
	plan  apiclient.AppImportRequest
	probe apiclient.AppImportSSHProbe
}

func sshImportAPI(t *testing.T, test apiclient.AppImportSSHResult) (*httptest.Server, *sshAPI) {
	t.Helper()
	rec := &sshAPI{}
	view := apiclient.AppImportView{ID: "appimp-9", Platform: "docker-ssh", States: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.calls = append(rec.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/ssh/host-keys"):
			_ = json.NewDecoder(r.Body).Decode(&rec.probe)
			_ = json.NewEncoder(w).Encode(apiclient.AppImportSSHResult{OK: true, Target: "h:22", Keys: []apiclient.AppImportSSHHostKey{{Type: "ssh-ed25519", Fingerprint: "SHA256:abc"}}})
		case strings.HasSuffix(r.URL.Path, "/ssh/test"):
			_ = json.NewDecoder(r.Body).Decode(&rec.probe)
			_ = json.NewEncoder(w).Encode(test)
		case strings.HasSuffix(r.URL.Path, "/volumes/copy"):
			_ = json.NewEncoder(w).Encode(apiclient.AppImportVolumeCopy{Results: []apiclient.AppImportVolumeCopyResult{
				{App: "web", ContainerPath: "/data", Volume: "app-web-data", State: "copied", Files: 3, Bytes: 2048},
				{App: "db", ContainerPath: "/var/lib/db", Volume: "app-db-data", State: "skipped", Fallback: "use the rsync command"}}})
		default:
			_ = json.NewDecoder(r.Body).Decode(&rec.plan)
			_ = json.NewEncoder(w).Encode(view)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func writeKey(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "id")
	if err := os.WriteFile(p, []byte(sshCLIKey), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportAppsDockerSSHWithoutFingerprintListsKeysAndStops(t *testing.T) {
	srv, rec := sshImportAPI(t, apiclient.AppImportSSHResult{})
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--from", "docker-ssh", "--ssh", "root@old.example.com:2222", "--ssh-key", writeKey(t), "--plan", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitUsage {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "SHA256:abc") || !strings.Contains(errb.String(), "--fingerprint") {
		t.Errorf("no fingerprint guidance: %s", errb.String())
	}
	if len(rec.calls) != 1 || !strings.HasSuffix(rec.calls[0], "/ssh/host-keys") {
		t.Errorf("calls = %v", rec.calls)
	}
	if rec.probe.SSH.PrivateKey != "" || rec.probe.SSH.Host != "old.example.com" || rec.probe.SSH.Port != 2222 {
		t.Errorf("the host key request carried %+v", rec.probe.SSH)
	}
}

func TestImportAppsDockerSSHPlanSendsLoginInBodyOnly(t *testing.T) {
	srv, rec := sshImportAPI(t, apiclient.AppImportSSHResult{})
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--from", "docker-ssh", "--ssh", "root@old.example.com", "--ssh-key", writeKey(t),
		"--fingerprint", "SHA256:pinned", "--ssh-socket", "/run/docker.sock", "--plan", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp", envImportSSHPassphrase: "pp"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	ssh := rec.plan.SSH
	if ssh == nil || ssh.Host != "old.example.com" || ssh.User != "root" || ssh.Fingerprint != "SHA256:pinned" || ssh.SocketPath != "/run/docker.sock" ||
		ssh.Passphrase != "pp" || !strings.Contains(ssh.PrivateKey, "secret-cli-key") {
		t.Fatalf("request ssh = %+v", ssh)
	}
	if rec.plan.Platform != "docker-ssh" || rec.plan.URL != "" || rec.plan.Token != "" {
		t.Errorf("request = %+v", rec.plan)
	}
	if strings.Contains(out.String()+errb.String(), "secret-cli-key") {
		t.Error("key printed")
	}
}

func TestImportAppsDockerSSHInsecureHostKeyWarns(t *testing.T) {
	srv, _ := sshImportAPI(t, apiclient.AppImportSSHResult{})
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--from", "docker-ssh", "--ssh", "root@h", "--ssh-key", writeKey(t), "--insecure-host-key", "--plan", "--api-url", srv.URL},
		&out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitOK || !strings.Contains(errb.String(), "warning: host key verification is off") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}

func TestImportAppsDockerSSHTestConnection(t *testing.T) {
	ok := apiclient.AppImportSSHResult{OK: true, Target: "root@h:22", Reads: "read only: lists containers, volumes and images",
		Overview: &apiclient.AppImportSSHOverview{DockerVersion: "27.3.1", OS: "linux", Containers: 5, Running: 4, Volumes: 3, Images: 7}}
	srv, _ := sshImportAPI(t, ok)
	var out, errb bytes.Buffer
	args := []string{"import", "apps", "--from", "docker-ssh", "--ssh", "root@h", "--ssh-key", writeKey(t), "--fingerprint", "SHA256:x", "--test-connection", "--api-url", srv.URL}
	if code := run("cli", args, &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"})); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, want := range []string{"read only: lists containers, volumes and images", "5 containers (4 running)", "3 volumes", "7 images"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q: %s", want, out.String())
		}
	}

	bad := apiclient.AppImportSSHResult{Kind: "socket_permission", Message: "denied"}
	srv2, _ := sshImportAPI(t, bad)
	out.Reset()
	errb.Reset()
	args[len(args)-1] = srv2.URL
	if code := run("cli", args, &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"})); code != exitCheckFailed {
		t.Fatalf("failure exit %d", code)
	}
	if !strings.Contains(errb.String(), "docker group") {
		t.Errorf("no typed fix: %s", errb.String())
	}
}

func TestImportAppsCopyVolumes(t *testing.T) {
	srv, rec := sshImportAPI(t, apiclient.AppImportSSHResult{})
	var out, errb bytes.Buffer
	code := run("cli", []string{"import", "apps", "--session", "appimp-9", "--copy-volumes", "--api-url", srv.URL}, &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if len(rec.calls) != 1 || rec.calls[0] != "POST /api/v1/migration/apps/sessions/appimp-9/volumes/copy" {
		t.Errorf("calls = %v", rec.calls)
	}
	for _, want := range []string{"app-web-data: copied (3 files", "use the rsync command"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q: %s", want, out.String())
		}
	}
	if code := run("cli", []string{"import", "apps", "--copy-volumes", "--api-url", srv.URL}, &out, &errb, importEnv(nil)); code != exitUsage {
		t.Errorf("without a session: exit %d", code)
	}
}

func TestImportAppsDockerSSHUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"import", "apps", "--from", "docker-ssh", "--plan"},
		{"import", "apps", "--from", "docker-ssh", "--ssh", "nohost", "--ssh-key", "-", "--plan"},
		{"import", "apps", "--from", "docker-ssh", "--ssh", "root@h", "--plan"},
	} {
		var out, errb bytes.Buffer
		if code := run("cli", args, &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"})); code != exitUsage {
			t.Errorf("%v: exit %d, want usage", args, code)
		}
	}
}

func TestImportAppsUsageMentionsEveryDockerSSHFlag(t *testing.T) {
	u := importAppsUsage("cli")
	for _, flag := range []string{"--from docker-ssh", "--ssh user@host", "--ssh-key", "--fingerprint", "--insecure-host-key", "--ssh-socket", "--test-connection", "--copy-volumes", "--allow-private"} {
		if !strings.Contains(u, flag) {
			t.Errorf("usage lacks %q", flag)
		}
	}
}
