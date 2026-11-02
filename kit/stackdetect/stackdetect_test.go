package stackdetect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  Stack
	}{
		{
			name:  "dockerfile with expose",
			files: map[string]string{"Dockerfile": "FROM alpine\nEXPOSE 9000/tcp\n", "package.json": "{}"},
			want:  Stack{Provider: "dockerfile", Build: "dockerfile", Path: "./Dockerfile", Port: 9000},
		},
		{
			name:  "dockerfile without expose",
			files: map[string]string{"Dockerfile": "FROM alpine\n"},
			want:  Stack{Provider: "dockerfile", Build: "dockerfile", Path: "./Dockerfile", Port: 8080},
		},
		{
			name:  "compose",
			files: map[string]string{"docker-compose.yml": "services: {}\n"},
			want:  Stack{Provider: "compose", Build: "compose", Path: "./docker-compose.yml"},
		},
		{
			name:  "next",
			files: map[string]string{"package.json": `{"dependencies":{"next":"15"},"scripts":{"start":"next start"}}`},
			want:  Stack{Provider: "node", Build: "railpack", Port: 3000, Label: "Node.js (Next.js)"},
		},
		{
			name:  "vite spa",
			files: map[string]string{"package.json": `{"devDependencies":{"vite":"6"}}`},
			want:  Stack{Provider: "node", Build: "railpack", Port: 3000, Label: "Node.js (Vite or Create React App)"},
		},
		{
			name:  "go",
			files: map[string]string{"go.mod": "module x\n"},
			want:  Stack{Provider: "golang", Build: "railpack", Port: 8080, Label: "Go"},
		},
		{
			name:  "java gradle",
			files: map[string]string{"build.gradle.kts": ""},
			want:  Stack{Provider: "java", Build: "railpack", Port: 8080, Label: "Java"},
		},
		{
			name:  "python fastapi",
			files: map[string]string{"requirements.txt": "FastAPI==0.1\n"},
			want:  Stack{Provider: "python", Build: "railpack", Port: 8000, Label: "Python (FastAPI)"},
		},
		{
			name:  "static",
			files: map[string]string{"index.html": "<html></html>"},
			want:  Stack{Provider: "static", Build: "static", Path: "./", Label: "Static site"},
		},
		{name: "ruby rails", files: map[string]string{"Gemfile": "gem 'rails'"}, want: Stack{Provider: "ruby", Build: "railpack", Port: 3000, Label: "Ruby (Rails)"}},
		{name: "php symfony", files: map[string]string{"composer.json": `{"require":{"symfony/framework-bundle":"7"}}`}, want: Stack{Provider: "php", Build: "railpack", Port: 80, Label: "PHP (Symfony)"}},
		{name: "rust axum", files: map[string]string{"Cargo.toml": "[dependencies]\naxum = \"0.8\""}, want: Stack{Provider: "rust", Build: "railpack", Port: 3000, Label: "Rust (axum)"}},
		{name: "elixir phoenix", files: map[string]string{"mix.exs": "{:phoenix, \"~> 1.7\"}"}, want: Stack{Provider: "elixir", Build: "railpack", Port: 4000, Label: "Elixir (Phoenix)"}},
		{name: "deno", files: map[string]string{"deno.json": "{}"}, want: Stack{Provider: "deno", Build: "railpack", Port: 8000}},
		{name: "dotnet", files: map[string]string{"App.csproj": "<Project/>"}, want: Stack{Provider: "dotnet", Build: "railpack", Port: 8080}},
		{name: "hugo", files: map[string]string{"hugo.toml": "title='x'"}, want: Stack{Provider: "hugo", Build: "static", Path: "./"}},
		{name: "jekyll", files: map[string]string{"_config.yml": "", "Gemfile": "gem 'jekyll'"}, want: Stack{Provider: "jekyll", Build: "static", Path: "./"}},
		{name: "eleventy", files: map[string]string{"package.json": `{"devDependencies":{"@11ty/eleventy":"3"}}`}, want: Stack{Provider: "eleventy", Build: "static", Path: "./"}},
		{name: "gatsby", files: map[string]string{"package.json": `{"dependencies":{"gatsby":"5"}}`}, want: Stack{Provider: "gatsby", Build: "static", Path: "./"}},
		{name: "angular", files: map[string]string{"package.json": `{"dependencies":{"@angular/core":"19"},"scripts":{"start":"ng serve"}}`}, want: Stack{Provider: "node", Build: "railpack", Port: 3000, Label: "Node.js (Angular)"}},
		{name: "astro ssr", files: map[string]string{"package.json": `{"dependencies":{"astro":"5","@astrojs/node":"9"},"scripts":{"start":"x"}}`}, want: Stack{Provider: "node", Build: "railpack", Port: 4321, Label: "Node.js (Astro SSR)"}},
		{name: "procfile only", files: map[string]string{"Procfile": "web: ./run.sh\n"}, want: Stack{Provider: "procfile", Build: "railpack", Port: 8080}},
		{
			name:  "nothing",
			files: map[string]string{"README.md": "hi"},
			want:  Stack{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "My_App")
			if err := os.Mkdir(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			for f, body := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Detect(dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if got.Name != "my-app" {
				t.Errorf("Name = %q, want my-app", got.Name)
			}
			if got.Provider != tc.want.Provider || got.Build != tc.want.Build || got.Path != tc.want.Path || got.Port != tc.want.Port {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if tc.want.Label != "" && got.Label != tc.want.Label {
				t.Errorf("Label = %q, want %q", got.Label, tc.want.Label)
			}
			if got.Detected() != (tc.want.Build != "") {
				t.Errorf("Detected() = %v", got.Detected())
			}
		})
	}
}

func TestDetect_BadPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(dir); err == nil {
		t.Fatal("want error for malformed package.json")
	}
}

func TestServiceName(t *testing.T) {
	for in, want := range map[string]string{"/x/My App!": "my-app", "/x/123abc": "web", "/x/api-v2": "api-v2", "/x/__": "web"} {
		if got := serviceName(in); got != want {
			t.Errorf("serviceName(%q) = %q, want %q", in, got, want)
		}
	}
}
