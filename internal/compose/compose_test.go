package compose

import "testing"

func TestParse_ValidFile(t *testing.T) {
	data := []byte(`
version: "3.8"
services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
    environment:
      - NODE_ENV=production
    volumes:
      - web-data:/usr/share/nginx/html
    labels:
      team: platform
  redis:
    image: redis:7
    environment:
      REDIS_PASSWORD: hunter2
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	web, ok := f.Services["web"]
	if !ok {
		t.Fatal("expected service web")
	}
	if web.Image != "nginx:1.27" {
		t.Errorf("web.Image = %q, want nginx:1.27", web.Image)
	}
	if len(web.Ports) != 1 || web.Ports[0].HostPort != 8080 || web.Ports[0].ContainerPort != 80 {
		t.Errorf("web.Ports = %+v, want [{8080 80}]", web.Ports)
	}
	if web.Environment["NODE_ENV"] != "production" {
		t.Errorf("web.Environment[NODE_ENV] = %q, want production", web.Environment["NODE_ENV"])
	}
	if len(web.Volumes) != 1 || web.Volumes[0].Name != "web-data" || web.Volumes[0].ContainerPath != "/usr/share/nginx/html" {
		t.Errorf("web.Volumes = %+v, want [{web-data /usr/share/nginx/html}]", web.Volumes)
	}
	if web.Labels["team"] != "platform" {
		t.Errorf("web.Labels[team] = %q, want platform", web.Labels["team"])
	}

	redis, ok := f.Services["redis"]
	if !ok {
		t.Fatal("expected service redis")
	}
	if redis.Environment["REDIS_PASSWORD"] != "hunter2" {
		t.Errorf("redis.Environment[REDIS_PASSWORD] = %q, want hunter2", redis.Environment["REDIS_PASSWORD"])
	}
}

func TestParse_Healthcheck_StringForm(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    healthcheck:
      test: curl -f http://localhost/health
      interval: 10s
      timeout: 2s
      retries: 3
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	hc := f.Services["web"].Healthcheck
	if hc == nil {
		t.Fatal("Healthcheck = nil, want a parsed block")
	}
	want := []string{"CMD-SHELL", "curl -f http://localhost/health"}
	if len(hc.Test) != 2 || hc.Test[0] != want[0] || hc.Test[1] != want[1] {
		t.Errorf("Test = %v, want %v (bare string implies CMD-SHELL)", hc.Test, want)
	}
	if hc.Interval != "10s" || hc.Timeout != "2s" || hc.Retries != 3 {
		t.Errorf("hc = %+v, want Interval=10s Timeout=2s Retries=3", hc)
	}
}

func TestParse_Healthcheck_ListForm(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	hc := f.Services["web"].Healthcheck
	if hc == nil || len(hc.Test) != 4 || hc.Test[0] != "CMD" {
		t.Errorf("Test = %+v, want [CMD curl -f http://localhost/health]", hc)
	}
}

func TestParse_NoHealthcheck_StaysNil(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if f.Services["web"].Healthcheck != nil {
		t.Errorf("Healthcheck = %+v, want nil", f.Services["web"].Healthcheck)
	}
}

func TestValidate_Invalid(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "no services",
			yaml: `services: {}`,
		},
		{
			name: "missing image",
			yaml: `
services:
  web:
    ports: ["80"]
`,
		},
		{
			name: "build is rejected",
			yaml: `
services:
  web:
    build: .
`,
		},
		{
			name: "bind mount is rejected",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - ./local:/data
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if err := f.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}

func TestParse_UnsupportedPortsForm(t *testing.T) {
	_, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
`))
	if err == nil {
		t.Fatal("Parse() error = nil, want an error for a long-form ports: entry")
	}
}

func TestParse_UnsupportedVolumesForm(t *testing.T) {
	_, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    volumes:
      - type: volume
        source: web-data
        target: /data
`))
	if err == nil {
		t.Fatal("Parse() error = nil, want an error for a long-form volumes: entry")
	}
}

func TestParse_EnvironmentListForm(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    environment:
      - FOO=bar
      - BARE_KEY
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	env := f.Services["web"].Environment
	if env["FOO"] != "bar" {
		t.Errorf("env[FOO] = %q, want bar", env["FOO"])
	}
	if v, ok := env["BARE_KEY"]; !ok || v != "" {
		t.Errorf("env[BARE_KEY] = %q, ok=%v, want empty string, ok=true", v, ok)
	}
}
