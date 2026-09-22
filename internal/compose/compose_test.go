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
			name: "deny-listed bind mount host path is rejected",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - /etc:/data
`,
		},
		{
			name: "docker socket bind mount is rejected",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
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

func TestParse_LongFormPorts(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
      - target: 443
        published: "8443"
        protocol: tcp
        mode: host
      - target: 9000
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	ports := f.Services["web"].Ports
	if len(ports) != 3 {
		t.Fatalf("len(Ports) = %d, want 3", len(ports))
	}
	if ports[0].HostPort != 8080 || ports[0].ContainerPort != 80 {
		t.Errorf("ports[0] = %+v, want {8080 80}", ports[0])
	}
	if ports[1].HostPort != 8443 || ports[1].ContainerPort != 443 {
		t.Errorf("ports[1] = %+v, want {8443 443} (published as a quoted string)", ports[1])
	}
	if ports[2].HostPort != 0 || ports[2].ContainerPort != 9000 {
		t.Errorf("ports[2] = %+v, want {0 9000} (no published: key)", ports[2])
	}
}

func TestParse_LongFormPorts_Rejections(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing target",
			yaml: `
services:
  web:
    image: nginx:1.27
    ports:
      - published: 8080
`,
		},
		{
			name: "udp protocol",
			yaml: `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 53
        protocol: udp
`,
		},
		{
			name: "published range",
			yaml: `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: "8000-8010"
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.yaml)); err == nil {
				t.Fatal("Parse() error = nil, want an error")
			}
		})
	}
}

func TestParse_LongFormVolumes(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    volumes:
      - type: volume
        source: web-data
        target: /data
      - type: volume
        source: cache
        target: /cache
        read_only: true
      - type: bind
        source: /srv/myapp/static
        target: /static
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	volumes := f.Services["web"].Volumes
	if len(volumes) != 3 {
		t.Fatalf("len(Volumes) = %d, want 3", len(volumes))
	}
	if volumes[0].Name != "web-data" || volumes[0].ContainerPath != "/data" || volumes[0].ReadOnly {
		t.Errorf("volumes[0] = %+v, want named volume web-data at /data, not read-only", volumes[0])
	}
	if volumes[1].Name != "cache" || volumes[1].ContainerPath != "/cache" || !volumes[1].ReadOnly {
		t.Errorf("volumes[1] = %+v, want named volume cache at /cache, read-only", volumes[1])
	}
	if volumes[2].HostPath != "/srv/myapp/static" || volumes[2].ContainerPath != "/static" {
		t.Errorf("volumes[2] = %+v, want bind mount /srv/myapp/static at /static", volumes[2])
	}
}

func TestParse_LongFormVolumes_Rejections(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing target",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - type: volume
        source: web-data
`,
		},
		{
			name: "volume type with no source",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - type: volume
        target: /data
`,
		},
		{
			name: "bind type with relative source",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - type: bind
        source: ./data
        target: /data
`,
		},
		{
			name: "tmpfs type is rejected",
			yaml: `
services:
  web:
    image: nginx:1.27
    volumes:
      - type: tmpfs
        target: /cache
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.yaml)); err == nil {
				t.Fatal("Parse() error = nil, want an error")
			}
		})
	}
}

func TestParse_RelativeBindMountPath_Rejected(t *testing.T) {
	_, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    volumes:
      - ./local:/data
`))
	if err == nil {
		t.Fatal("Parse() error = nil, want an error for a relative bind-mount path")
	}
}

func TestParse_AbsoluteBindMountVolume(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    volumes:
      - /srv/myapp/data:/data
      - /srv/myapp/config:/config:ro
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	web := f.Services["web"]
	if len(web.Volumes) != 2 {
		t.Fatalf("web.Volumes = %+v, want 2 entries", web.Volumes)
	}
	rw := web.Volumes[0]
	if rw.Name != "" || rw.HostPath != "/srv/myapp/data" || rw.ContainerPath != "/data" || rw.ReadOnly {
		t.Errorf("web.Volumes[0] = %+v, want HostPath=/srv/myapp/data ContainerPath=/data ReadOnly=false", rw)
	}
	ro := web.Volumes[1]
	if ro.Name != "" || ro.HostPath != "/srv/myapp/config" || ro.ContainerPath != "/config" || !ro.ReadOnly {
		t.Errorf("web.Volumes[1] = %+v, want HostPath=/srv/myapp/config ContainerPath=/config ReadOnly=true", ro)
	}

	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want an absolute bind-mount path to be accepted", err)
	}
}

func TestValidate_ForbiddenBindMountPaths(t *testing.T) {
	tests := []string{
		"/", "/etc", "/root", "/boot", "/sys", "/proc",
		"/var/lib/docker", "/var/run/docker.sock", "/var/run", "/var/run/subdir",
	}
	for _, hostPath := range tests {
		t.Run(hostPath, func(t *testing.T) {
			f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    volumes:
      - "` + hostPath + `:/data"
`))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if err := f.Validate(); err == nil {
				t.Fatalf("Validate() error = nil, want %q to be rejected even for a root caller", hostPath)
			}
		})
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

func TestParse_Command(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{
			name: "string form wraps as sh -c",
			yaml: "command: server /data",
			want: []string{"/bin/sh", "-c", "server /data"},
		},
		{
			name: "list form passes through",
			yaml: `command: ["server", "/data", "--console-address", ":9001"]`,
			want: []string{"server", "/data", "--console-address", ":9001"},
		},
		{
			name: "absent stays nil",
			yaml: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := "services:\n  web:\n    image: nginx:1.27\n"
			if tt.yaml != "" {
				doc += "    " + tt.yaml + "\n"
			}
			f, err := Parse([]byte(doc))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got := []string(f.Services["web"].Command)
			if len(got) != len(tt.want) {
				t.Fatalf("Command = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Command[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParse_Entrypoint(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{
			name: "string form wraps as sh -c",
			yaml: "entrypoint: docker-entrypoint.sh",
			want: []string{"/bin/sh", "-c", "docker-entrypoint.sh"},
		},
		{
			name: "list form passes through",
			yaml: `entrypoint: ["docker-entrypoint.sh", "-c", "postgresql.conf"]`,
			want: []string{"docker-entrypoint.sh", "-c", "postgresql.conf"},
		},
		{
			name: "absent stays nil",
			yaml: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := "services:\n  web:\n    image: nginx:1.27\n"
			if tt.yaml != "" {
				doc += "    " + tt.yaml + "\n"
			}
			f, err := Parse([]byte(doc))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got := []string(f.Services["web"].Entrypoint)
			if len(got) != len(tt.want) {
				t.Fatalf("Entrypoint = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Entrypoint[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestParse_PullPolicy checks Parse's own raw, pre-normalization field:
// Parse never normalizes pull_policy:'s value, only normalizePullPolicy
// (exercised by TestNormalizePullPolicy below) and, in turn,
// ToDesiredServices (TestToDesiredServices_PullPolicy) do.
func TestParse_PullPolicy(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "absent stays empty", yaml: "", want: ""},
		{name: "always", yaml: "pull_policy: always", want: "always"},
		{name: "missing stays raw", yaml: "pull_policy: missing", want: "missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := "services:\n  web:\n    image: nginx:1.27\n"
			if tt.yaml != "" {
				doc += "    " + tt.yaml + "\n"
			}
			f, err := Parse([]byte(doc))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got := f.Services["web"].PullPolicy; got != tt.want {
				t.Errorf("PullPolicy = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePullPolicy(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty defaults", raw: "", want: ""},
		{name: "missing normalizes to empty", raw: "missing", want: ""},
		{name: "if_not_present normalizes to empty", raw: "if_not_present", want: ""},
		{name: "always", raw: "always", want: "always"},
		{name: "never is rejected", raw: "never", wantErr: true},
		{name: "build is rejected", raw: "build", wantErr: true},
		{name: "typo is rejected", raw: "Always", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePullPolicy(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizePullPolicy(%q) error = nil, want an error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizePullPolicy(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("normalizePullPolicy(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
