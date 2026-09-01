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

	"github.com/GLINCKER/levelrail/internal/spec"
)

// scriptedStdin joins lines with "\n" into the multi-answer transcript
// runInteractiveWizard reads one line per prompt from.
func scriptedStdin(lines ...string) *strings.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

func TestWizardPrompter_ReadRequired(t *testing.T) {
	var stderr bytes.Buffer
	p := newWizardPrompter(scriptedStdin("", "", "value"), &stderr)
	got, err := p.readRequired("prompt: ")
	if err != nil {
		t.Fatalf("readRequired() error = %v", err)
	}
	if got != "value" {
		t.Errorf("readRequired() = %q, want %q", got, "value")
	}
	if !strings.Contains(stderr.String(), "required") {
		t.Errorf("stderr = %q, want a hint about the blank answers being rejected", stderr.String())
	}
}

func TestWizardPrompter_ReadOptional(t *testing.T) {
	tests := []struct {
		name  string
		input string
		def   string
		want  string
	}{
		{name: "blank uses default", input: "", def: "fallback", want: "fallback"},
		{name: "value overrides default", input: "custom", def: "fallback", want: "custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newWizardPrompter(scriptedStdin(tt.input), &bytes.Buffer{})
			got, err := p.readOptional("prompt: ", tt.def)
			if err != nil {
				t.Fatalf("readOptional() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("readOptional() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWizardPrompter_ReadInt(t *testing.T) {
	p := newWizardPrompter(scriptedStdin("abc", "-5", "0", "8080"), &bytes.Buffer{})
	got, err := p.readInt("prompt: ")
	if err != nil {
		t.Fatalf("readInt() error = %v", err)
	}
	if got != 8080 {
		t.Errorf("readInt() = %d, want 8080 (non-positive/non-numeric answers should be retried)", got)
	}
}

func TestWizardPrompter_ReadChoice(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		def   string
		want  string
	}{
		{name: "blank uses default", input: []string{""}, def: "file", want: "file"},
		{name: "case insensitive match", input: []string{"API"}, def: "file", want: "api"},
		{name: "invalid retried", input: []string{"bogus", "api"}, def: "file", want: "api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newWizardPrompter(scriptedStdin(tt.input...), &bytes.Buffer{})
			got, err := p.readChoice("prompt: ", tt.def, "file", "api")
			if err != nil {
				t.Fatalf("readChoice() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("readChoice() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWizardPrompter_ReadLine_EOF(t *testing.T) {
	p := newWizardPrompter(strings.NewReader(""), &bytes.Buffer{})
	_, err := p.readLine("prompt: ")
	if err == nil {
		t.Fatal("readLine() error = nil, want an error on empty stdin (no answer ever given)")
	}
	if _, ok := err.(*validationError); !ok {
		t.Errorf("err type = %T, want *validationError (refuse, don't hang, matching authprompt.go's own EOF handling)", err)
	}
}

func TestRunInteractiveWizard(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		detected detectedGit
		wantErr  string
		want     func(t *testing.T, a wizardAnswers)
	}{
		{
			name:  "image source, file output, minimal answers",
			lines: []string{"My App", "image", "registry.example.com/org/app:v1", "8080", "", "", "", "", "file"},
			want: func(t *testing.T, a wizardAnswers) {
				if a.serviceName != "my-app" {
					t.Errorf("serviceName = %q, want %q", a.serviceName, "my-app")
				}
				if a.sourceKind != wizardSourceImage {
					t.Errorf("sourceKind = %v, want wizardSourceImage", a.sourceKind)
				}
				if a.image != "registry.example.com/org/app:v1" {
					t.Errorf("image = %q", a.image)
				}
				if a.port != 8080 {
					t.Errorf("port = %d, want 8080", a.port)
				}
				if a.domain != "" {
					t.Errorf("domain = %q, want empty (skipped)", a.domain)
				}
				if a.healthPath != "/healthz" {
					t.Errorf("healthPath = %q, want default /healthz", a.healthPath)
				}
				if a.memory != "" || a.cpu != 0 {
					t.Errorf("memory/cpu = %q/%v, want no limits", a.memory, a.cpu)
				}
				if a.outputMode != wizardOutputFile {
					t.Errorf("outputMode = %v, want wizardOutputFile", a.outputMode)
				}
			},
		},
		{
			name:     "git source auto-detected repo, health skipped, resources set, api output",
			lines:    []string{"webapp", "git", "", "3000", "app.example.com", "skip", "512Mi", "0.5", "api", "registry.example.com/org/webapp"},
			detected: detectedGit{RepoURL: "https://example.com/detected.git", Ref: "feature"},
			want: func(t *testing.T, a wizardAnswers) {
				if a.sourceKind != wizardSourceGit {
					t.Errorf("sourceKind = %v, want wizardSourceGit", a.sourceKind)
				}
				if a.repoURL != "https://example.com/detected.git" {
					t.Errorf("repoURL = %q, want auto-detected repo when blank was entered", a.repoURL)
				}
				if a.ref != "feature" {
					t.Errorf("ref = %q, want detected branch %q", a.ref, "feature")
				}
				if a.domain != "app.example.com" {
					t.Errorf("domain = %q", a.domain)
				}
				if a.healthPath != "" {
					t.Errorf("healthPath = %q, want empty ('skip' was entered)", a.healthPath)
				}
				if a.memory != "512Mi" || a.cpu != 0.5 {
					t.Errorf("memory/cpu = %q/%v, want 512Mi/0.5", a.memory, a.cpu)
				}
				if a.outputMode != wizardOutputAPI {
					t.Errorf("outputMode = %v, want wizardOutputAPI", a.outputMode)
				}
				if a.imageRepo != "registry.example.com/org/webapp" {
					t.Errorf("imageRepo = %q", a.imageRepo)
				}
			},
		},
		{
			name:  "custom health path is used verbatim",
			lines: []string{"app", "image", "img:1", "80", "", "/healthcheck", "", "", "file"},
			want: func(t *testing.T, a wizardAnswers) {
				if a.healthPath != "/healthcheck" {
					t.Errorf("healthPath = %q, want %q", a.healthPath, "/healthcheck")
				}
			},
		},
		{
			name:  "invalid source choice retried",
			lines: []string{"app", "bogus", "image", "img:1", "80", "", "skip", "", "", "file"},
			want: func(t *testing.T, a wizardAnswers) {
				if a.sourceKind != wizardSourceImage {
					t.Errorf("sourceKind = %v, want wizardSourceImage after retry", a.sourceKind)
				}
			},
		},
		{
			name:    "git source with no repo and nothing detected is rejected",
			lines:   []string{"app", "git", ""},
			wantErr: "git repository URL is required",
		},
		{
			name:    "EOF before the wizard finishes",
			lines:   []string{"app", "image"},
			wantErr: "read input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newWizardPrompter(scriptedStdin(tt.lines...), &bytes.Buffer{})
			got, err := runInteractiveWizard(p, tt.detected)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("err = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			tt.want(t, got)
		})
	}
}

func TestWizardAnswers_ToSpec(t *testing.T) {
	tests := []struct {
		name    string
		answers wizardAnswers
		wantErr string
		want    func(t *testing.T, s *spec.Spec)
	}{
		{
			name:    "image source minimal",
			answers: wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "img:1", port: 8080},
			want: func(t *testing.T, s *spec.Spec) {
				svc := s.Services["web"]
				if svc.Build.Type != spec.BuildImage || svc.Build.Image != "img:1" {
					t.Errorf("Build = %+v, want type=image image=img:1", svc.Build)
				}
				if svc.Health != nil {
					t.Errorf("Health = %+v, want nil when healthPath is empty", svc.Health)
				}
				if svc.Resources != nil {
					t.Errorf("Resources = %+v, want nil when no limits set", svc.Resources)
				}
			},
		},
		{
			name: "git source with domain, health, and resources",
			answers: wizardAnswers{
				serviceName: "web", sourceKind: wizardSourceGit, port: 3000,
				domain: "app.example.com", healthPath: "/healthz", memory: "512Mi", cpu: 0.5,
			},
			want: func(t *testing.T, s *spec.Spec) {
				svc := s.Services["web"]
				if svc.Build.Type != spec.BuildDockerfile {
					t.Errorf("Build.Type = %q, want dockerfile", svc.Build.Type)
				}
				if len(svc.Domains) != 1 || svc.Domains[0] != "app.example.com" {
					t.Errorf("Domains = %v, want [app.example.com]", svc.Domains)
				}
				if svc.Health == nil || svc.Health.Readiness == nil || svc.Health.Readiness.Path != "/healthz" {
					t.Fatalf("Health = %+v, want a readiness probe at /healthz", svc.Health)
				}
				if svc.Health.Liveness == nil || svc.Health.Liveness.Path != "/healthz" {
					t.Errorf("Health.Liveness = %+v, want the same probe as readiness", svc.Health.Liveness)
				}
				if svc.Resources == nil || svc.Resources.Memory != "512Mi" || svc.Resources.CPU != 0.5 {
					t.Errorf("Resources = %+v, want memory=512Mi cpu=0.5", svc.Resources)
				}
			},
		},
		{
			name:    "invalid: no port set",
			answers: wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "img:1"},
			wantErr: "port is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := tt.answers.toSpec()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("err = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			tt.want(t, s)
		})
	}
}

func TestWizardAnswers_AppYAML(t *testing.T) {
	a := wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "img:1", port: 8080, healthPath: "/healthz"}
	data, err := a.appYAML()
	if err != nil {
		t.Fatalf("appYAML() error = %v", err)
	}
	if _, err := spec.Parse(data); err != nil {
		t.Fatalf("generated app.yaml failed spec.Parse: %v (data=%s)", err, data)
	}
	if !strings.Contains(string(data), "img:1") {
		t.Errorf("app.yaml = %s, want it to contain the image reference", data)
	}
}

func TestWizardAnswers_AppYAML_PropagatesToSpecError(t *testing.T) {
	a := wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "img:1"} // no port
	if _, err := a.appYAML(); err == nil {
		t.Fatal("appYAML() error = nil, want the underlying toSpec validation error to surface")
	}
}

func TestWizardAnswers_ToCreatePlan(t *testing.T) {
	tests := []struct {
		name    string
		answers wizardAnswers
		wantErr string
		want    func(t *testing.T, p createPlan)
	}{
		{
			name:    "image source has no build trigger",
			answers: wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "registry.example.com/org/app:v1", port: 8080},
			want: func(t *testing.T, p createPlan) {
				if p.Build != nil {
					t.Errorf("Build = %+v, want nil for the image source", p.Build)
				}
				if p.CreateBody.Image != "registry.example.com/org/app:v1" {
					t.Errorf("CreateBody.Image = %q", p.CreateBody.Image)
				}
			},
		},
		{
			name: "git source builds a dockerfile build trigger, reusing planFromFile",
			answers: wizardAnswers{
				serviceName: "web", sourceKind: wizardSourceGit, port: 3000,
				repoURL: "https://example.com/x.git", ref: "release", imageRepo: "registry.example.com/org/web",
			},
			want: func(t *testing.T, p createPlan) {
				if p.Build == nil {
					t.Fatalf("Build = nil, want non-nil for the git source")
				}
				if p.Build.RepoURL != "https://example.com/x.git" || p.Build.Ref != "release" || p.Build.ImageRepo != "registry.example.com/org/web" {
					t.Errorf("Build = %+v, want repo/ref/imageRepo carried through", p.Build)
				}
				if p.Build.Build.Type != spec.BuildDockerfile {
					t.Errorf("Build.Build.Type = %q, want dockerfile", p.Build.Build.Type)
				}
				if p.CreateBody.Image != "registry.example.com/org/web:pending" {
					t.Errorf("CreateBody.Image = %q, want the pending placeholder", p.CreateBody.Image)
				}
			},
		},
		{
			name:    "git source missing image repo surfaces planFromFile's own error",
			answers: wizardAnswers{serviceName: "web", sourceKind: wizardSourceGit, port: 3000, repoURL: "https://example.com/x.git"},
			wantErr: "--image-repo is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := tt.answers.toCreatePlan()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("err = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			tt.want(t, p)
		})
	}
}

func TestRunWizardWriteFile(t *testing.T) {
	t.Run("writes a valid app.yaml", func(t *testing.T) {
		t.Chdir(t.TempDir())
		a := wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "img:1", port: 8080}
		var stdout, stderr bytes.Buffer
		got := runWizardWriteFile(a, &stdout, &stderr, false)
		if got != exitOK {
			t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
		}
		data, err := os.ReadFile(wizardAppYAMLPath)
		if err != nil {
			t.Fatalf("read %s: %v", wizardAppYAMLPath, err)
		}
		if _, err := spec.Parse(data); err != nil {
			t.Fatalf("written app.yaml failed spec.Parse: %v", err)
		}
	})

	t.Run("refuses to overwrite an existing file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if err := os.WriteFile(wizardAppYAMLPath, []byte("existing"), 0o600); err != nil {
			t.Fatalf("seed existing file: %v", err)
		}
		a := wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "img:1", port: 8080}
		var stdout, stderr bytes.Buffer
		got := runWizardWriteFile(a, &stdout, &stderr, false)
		if got != exitValidation {
			t.Fatalf("exit = %d, want %d", got, exitValidation)
		}
		if !strings.Contains(stderr.String(), "already exists") {
			t.Errorf("stderr = %q, want a refuse-to-overwrite message", stderr.String())
		}
		data, err := os.ReadFile(wizardAppYAMLPath)
		if err != nil {
			t.Fatalf("read %s: %v", wizardAppYAMLPath, err)
		}
		if string(data) != "existing" {
			t.Errorf("file content = %q, want it untouched", data)
		}
	})
}

func TestRunWizardCreateViaAPI(t *testing.T) {
	t.Run("image source: create only, no build triggered", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "registry.example.com/org/app:v1", Port: 8080})
		}))
		defer srv.Close()

		a := wizardAnswers{serviceName: "web", sourceKind: wizardSourceImage, image: "registry.example.com/org/app:v1", port: 8080}
		var stdout, stderr bytes.Buffer
		got := runWizardCreateViaAPI(a, &stdout, &stderr, credentialFlags{Token: "tok", APIURL: srv.URL}, false, envMap(), "levelrail-cli-test")
		if got != exitOK {
			t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
		}
		want := []string{"POST /api/v1/apps", "GET /api/v1/apps/web"}
		if len(paths) != len(want) {
			t.Fatalf("requests = %v, want %v", paths, want)
		}
		for i := range want {
			if paths[i] != want[i] {
				t.Errorf("requests[%d] = %q, want %q", i, paths[i], want[i])
			}
		}
	})

	t.Run("git source: create then trigger a build", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/builds"):
				_ = json.NewEncoder(w).Encode(buildTriggerResponse{Image: "registry.example.com/org/web:pending"})
			default:
				_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "registry.example.com/org/web:pending", Port: 3000})
			}
		}))
		defer srv.Close()

		a := wizardAnswers{
			serviceName: "web", sourceKind: wizardSourceGit, port: 3000,
			repoURL: "https://example.com/x.git", ref: "main", imageRepo: "registry.example.com/org/web",
		}
		var stdout, stderr bytes.Buffer
		got := runWizardCreateViaAPI(a, &stdout, &stderr, credentialFlags{Token: "tok", APIURL: srv.URL}, false, envMap(), "levelrail-cli-test")
		if got != exitOK {
			t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
		}
		want := []string{"POST /api/v1/apps", "POST /api/v1/apps/web/builds", "GET /api/v1/apps/web"}
		if len(paths) != len(want) {
			t.Fatalf("requests = %v, want %v", paths, want)
		}
		for i := range want {
			if paths[i] != want[i] {
				t.Errorf("requests[%d] = %q, want %q", i, paths[i], want[i])
			}
		}
	})

	t.Run("invalid answers never reach the network", func(t *testing.T) {
		a := wizardAnswers{serviceName: "web", sourceKind: wizardSourceGit, port: 3000, repoURL: "https://example.com/x.git"} // no imageRepo
		var stdout, stderr bytes.Buffer
		got := runWizardCreateViaAPI(a, &stdout, &stderr, credentialFlags{Token: "tok", APIURL: "http://127.0.0.1:0"}, false, envMap(), "levelrail-cli-test")
		if got != exitValidation {
			t.Fatalf("exit = %d, want %d", got, exitValidation)
		}
	})
}

func TestRunAppsCreate_InteractiveMutuallyExclusiveWithOtherModeFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := runAppsCreate("levelrail-cli-test", []string{"--interactive", "--name", "web"}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("stderr = %q, want a mutual-exclusion message", stderr.String())
	}
}

func TestRunAppsCreate_InteractiveEndToEnd_FileMode(t *testing.T) {
	t.Chdir(t.TempDir())
	stdin := scriptedStdin("myapp", "image", "registry.example.com/org/app:v1", "8080", "", "", "", "", "file")
	var stdout, stderr bytes.Buffer
	got := runAppsCreate("levelrail-cli-test", []string{"--interactive"}, &stdout, &stderr, envMap(), stdin)
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(".", wizardAppYAMLPath)); err != nil {
		t.Fatalf("expected %s to be written: %v", wizardAppYAMLPath, err)
	}
}

func TestParseCreateFlags_Interactive(t *testing.T) {
	var token, apiURL, profile string
	f, err := parseCreateFlags("levelrail", []string{"-i"}, &strings.Builder{}, &token, &apiURL, &profile)
	if err != nil {
		t.Fatalf("parseCreateFlags() error = %v", err)
	}
	if !f.interactive {
		t.Error("interactive = false, want true after -i")
	}
}
