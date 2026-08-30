package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testEngines() []databaseEngineResource {
	return []databaseEngineResource{
		{ID: "postgres", Label: "Postgres", DefaultVersion: "16"},
		{ID: "redis", Label: "Redis", DefaultVersion: "7"},
		{ID: "customdb", Label: "Custom", DefaultVersion: ""},
	}
}

func TestRunInteractiveDatabaseWizard(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		wantErr string
		want    func(t *testing.T, a databaseWizardAnswers)
	}{
		{
			name:  "minimal answers, everything skipped",
			lines: []string{"mydb", "postgres", "", "", "", "", ""},
			want: func(t *testing.T, a databaseWizardAnswers) {
				if a.name != "mydb" {
					t.Errorf("name = %q, want %q", a.name, "mydb")
				}
				if a.engine != "postgres" {
					t.Errorf("engine = %q, want %q", a.engine, "postgres")
				}
				if a.version != "16" {
					t.Errorf("version = %q, want the registry default %q", a.version, "16")
				}
				if a.memory != "" || a.cpu != 0 {
					t.Errorf("memory/cpu = %q/%v, want no limits", a.memory, a.cpu)
				}
				if a.public {
					t.Errorf("public = true, want false (default)")
				}
				if a.backupTargetID != "" {
					t.Errorf("backupTargetID = %q, want empty (skipped)", a.backupTargetID)
				}
			},
		},
		{
			name: "full answers: explicit engine, resources, public port, backup schedule",
			lines: []string{
				"cache", "redis", "", "512Mi", "0.5",
				"yes", "6380",
				"tgt1", "", "5", "",
			},
			want: func(t *testing.T, a databaseWizardAnswers) {
				if a.engine != "redis" || a.version != "7" {
					t.Errorf("engine/version = %q/%q, want redis/7", a.engine, a.version)
				}
				if a.memory != "512Mi" || a.cpu != 0.5 {
					t.Errorf("memory/cpu = %q/%v, want 512Mi/0.5", a.memory, a.cpu)
				}
				if !a.public || a.publicPort != 6380 {
					t.Errorf("public/publicPort = %v/%d, want true/6380", a.public, a.publicPort)
				}
				if a.backupTargetID != "tgt1" {
					t.Errorf("backupTargetID = %q, want %q", a.backupTargetID, "tgt1")
				}
				if a.backupSchedule != "0 3 * * *" {
					t.Errorf("backupSchedule = %q, want the default %q", a.backupSchedule, "0 3 * * *")
				}
				if a.backupRetain != 5 {
					t.Errorf("backupRetain = %d, want 5", a.backupRetain)
				}
				if a.backupRetainDays != 0 {
					t.Errorf("backupRetainDays = %d, want 0 (skipped)", a.backupRetainDays)
				}
			},
		},
		{
			name:  "invalid engine choice is retried",
			lines: []string{"db1", "bogus", "postgres", "", "", "", "", ""},
			want: func(t *testing.T, a databaseWizardAnswers) {
				if a.engine != "postgres" {
					t.Errorf("engine = %q, want postgres after retry", a.engine)
				}
			},
		},
		{
			name:  "public access left at default (no) skips the port prompt",
			lines: []string{"db2", "postgres", "", "", "", "no", ""},
			want: func(t *testing.T, a databaseWizardAnswers) {
				if a.public {
					t.Errorf("public = true, want false")
				}
				if a.publicPort != 0 {
					t.Errorf("publicPort = %d, want 0", a.publicPort)
				}
			},
		},
		{
			name:  "public access yes with blank port auto-assigns",
			lines: []string{"db3", "postgres", "", "", "", "yes", "", ""},
			want: func(t *testing.T, a databaseWizardAnswers) {
				if !a.public {
					t.Errorf("public = false, want true")
				}
				if a.publicPort != 0 {
					t.Errorf("publicPort = %d, want 0 (auto-assign)", a.publicPort)
				}
			},
		},
		{
			name:  "invalid memory value is retried",
			lines: []string{"db4", "postgres", "", "bogus", "256Mi", "", "", ""},
			want: func(t *testing.T, a databaseWizardAnswers) {
				if a.memory != "256Mi" {
					t.Errorf("memory = %q, want %q after retry", a.memory, "256Mi")
				}
			},
		},
		{
			name:    "engine with no registry default version and a blank answer is rejected",
			lines:   []string{"db5", "customdb", ""},
			wantErr: "version is required",
		},
		{
			name:    "EOF before the wizard finishes",
			lines:   []string{"db6"},
			wantErr: "read input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newWizardPrompter(scriptedStdin(tt.lines...), &bytes.Buffer{})
			got, err := runInteractiveDatabaseWizard(p, testEngines())
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

func TestDatabaseWizardAnswers_ToCreatePlan(t *testing.T) {
	tests := []struct {
		name    string
		answers databaseWizardAnswers
		wantErr string
		want    databaseResource
	}{
		{
			name:    "valid",
			answers: databaseWizardAnswers{name: "main", engine: "postgres", version: "16"},
			want:    databaseResource{Name: "main", Engine: "postgres", Version: "16"},
		},
		{
			name:    "reuses planDatabaseCreate's own validation for a missing engine",
			answers: databaseWizardAnswers{name: "main", version: "16"},
			wantErr: "--engine",
		},
		{
			name:    "reuses planDatabaseCreate's own validation for an unsupported engine",
			answers: databaseWizardAnswers{name: "main", engine: "cassandra", version: "16"},
			wantErr: "--engine must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.answers.toCreatePlan()
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
			if got != tt.want {
				t.Errorf("toCreatePlan() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDatabaseWizardAnswers_ToResources(t *testing.T) {
	t.Run("no limits requested", func(t *testing.T) {
		a := databaseWizardAnswers{name: "main", engine: "postgres", version: "16"}
		got, err := a.toResources()
		if err != nil {
			t.Fatalf("toResources() error = %v", err)
		}
		if got != nil {
			t.Errorf("toResources() = %+v, want nil", got)
		}
	})

	t.Run("memory and cpu, reusing toServiceResources' own conversion", func(t *testing.T) {
		a := databaseWizardAnswers{memory: "512Mi", cpu: 0.5}
		got, err := a.toResources()
		if err != nil {
			t.Fatalf("toResources() error = %v", err)
		}
		if got == nil {
			t.Fatal("toResources() = nil, want a resources value")
		}
		if got.MemoryBytes != 512*1024*1024 {
			t.Errorf("MemoryBytes = %d, want %d", got.MemoryBytes, 512*1024*1024)
		}
		if got.NanoCPUs != 500_000_000 {
			t.Errorf("NanoCPUs = %d, want %d", got.NanoCPUs, 500_000_000)
		}
	})

	t.Run("invalid memory value surfaces parseMemoryBytes' own error", func(t *testing.T) {
		a := databaseWizardAnswers{memory: "not-a-size"}
		if _, err := a.toResources(); err == nil {
			t.Fatal("toResources() error = nil, want an error for an unparseable memory value")
		}
	})
}

func TestCreateDatabaseFromWizard(t *testing.T) {
	t.Run("minimal answers: create then re-fetch only", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(databaseResource{Name: "mydb", Engine: "postgres", Version: "16"})
		}))
		defer srv.Close()

		a := databaseWizardAnswers{name: "mydb", engine: "postgres", version: "16"}
		client := NewClient(srv.URL, "tok")
		var stdout, stderr bytes.Buffer
		got := createDatabaseFromWizard(context.Background(), client, a, &stdout, &stderr, false)
		if got != exitOK {
			t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
		}
		want := []string{"POST /api/v1/databases", "GET /api/v1/databases/mydb"}
		if len(paths) != len(want) {
			t.Fatalf("requests = %v, want %v", paths, want)
		}
		for i := range want {
			if paths[i] != want[i] {
				t.Errorf("requests[%d] = %q, want %q", i, paths[i], want[i])
			}
		}
	})

	t.Run("full answers: resources, public access, and backup schedule all applied", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.HasSuffix(r.URL.Path, "/resources"):
				_ = json.NewEncoder(w).Encode(databaseResource{Name: "cache", Engine: "redis", Version: "7"})
			case strings.HasSuffix(r.URL.Path, "/public-access"):
				_ = json.NewEncoder(w).Encode(databasePublicAccessResource{DatabaseName: "cache", PubliclyAccessible: true, PublicPort: 6380})
			case strings.HasSuffix(r.URL.Path, "/backup-schedule"):
				_ = json.NewEncoder(w).Encode(backupScheduleResource{DatabaseName: "cache", TargetID: "tgt1", Schedule: "0 3 * * *"})
			default:
				_ = json.NewEncoder(w).Encode(databaseResource{Name: "cache", Engine: "redis", Version: "7"})
			}
		}))
		defer srv.Close()

		a := databaseWizardAnswers{
			name: "cache", engine: "redis", version: "7",
			memory: "256Mi", cpu: 0.25,
			public: true, publicPort: 6380,
			backupTargetID: "tgt1", backupSchedule: "0 3 * * *", backupRetain: 5,
		}
		client := NewClient(srv.URL, "tok")
		var stdout, stderr bytes.Buffer
		got := createDatabaseFromWizard(context.Background(), client, a, &stdout, &stderr, false)
		if got != exitOK {
			t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
		}
		want := []string{
			"POST /api/v1/databases",
			"PUT /api/v1/databases/cache/resources",
			"PUT /api/v1/databases/cache/public-access",
			"PUT /api/v1/databases/cache/backup-schedule",
			"GET /api/v1/databases/cache",
		}
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
		a := databaseWizardAnswers{name: "mydb", version: "16"} // no engine
		client := NewClient("http://127.0.0.1:0", "tok")
		var stdout, stderr bytes.Buffer
		got := createDatabaseFromWizard(context.Background(), client, a, &stdout, &stderr, false)
		if got != exitValidation {
			t.Fatalf("exit = %d, want %d", got, exitValidation)
		}
	})

	t.Run("create failure stops before any follow-up call", func(t *testing.T) {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.Method+" "+r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		a := databaseWizardAnswers{name: "mydb", engine: "postgres", version: "16", public: true}
		client := NewClient(srv.URL, "tok")
		var stdout, stderr bytes.Buffer
		got := createDatabaseFromWizard(context.Background(), client, a, &stdout, &stderr, false)
		if got == exitOK {
			t.Fatalf("exit = %d, want a non-OK exit code", got)
		}
		if len(paths) != 1 || paths[0] != "POST /api/v1/databases" {
			t.Fatalf("requests = %v, want only the initial create call", paths)
		}
	})
}

func TestRunDatabasesCreateWizard_EndToEnd(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/database-engines":
			_ = json.NewEncoder(w).Encode(testEngines())
		default:
			_ = json.NewEncoder(w).Encode(databaseResource{Name: "mydb", Engine: "postgres", Version: "16"})
		}
	}))
	defer srv.Close()

	stdin := scriptedStdin("mydb", "postgres", "", "", "", "", "")
	var stdout, stderr bytes.Buffer
	got := runDatabasesCreateWizard(stdin, &stdout, &stderr, "tok", srv.URL, false, envMap(), "levelrail-cli-test")
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if len(paths) == 0 || paths[0] != "GET /api/v1/database-engines" {
		t.Fatalf("requests = %v, want the engine registry fetched first", paths)
	}
	if !strings.Contains(stdout.String()+stderr.String(), "mydb") {
		t.Errorf("output = %q, want it to mention the created database", stdout.String()+stderr.String())
	}
}

func TestRunDatabasesCreate_InteractiveMutuallyExclusiveWithOtherModeFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := runDatabasesCreate("levelrail-cli-test", []string{"--interactive", "--name", "mydb"}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "cannot be combined") {
		t.Errorf("stderr = %q, want a mutual-exclusion message", stderr.String())
	}
}
