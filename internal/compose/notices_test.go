package compose

import "testing"

func TestNotices(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		wants []Notice
	}{
		{
			name: "no restart, no networks: no notices",
			yaml: `
services:
  web:
    image: nginx:latest
`,
		},
		{
			name: "restart: no is the platform's own default: no note",
			yaml: `
services:
  web:
    image: nginx:latest
    restart: "no"
`,
		},
		{
			name: "restart: always notes the reconciler owns lifecycle",
			yaml: `
services:
  web:
    image: nginx:latest
    restart: always
`,
			wants: []Notice{
				{Level: NoticeLevelNote, Message: `service "web": restart: "always" is parsed but not applied; Levelrail's reconciler keeps containers running, not Docker's native restart policy`},
			},
		},
		{
			name: "top-level networks: warns even with no per-service assignment",
			yaml: `
services:
  web:
    image: nginx:latest
networks:
  frontend: {}
`,
			wants: []Notice{
				{Level: NoticeLevelWarning, Message: "networks: is declared but not enforced; every service in this app shares one network and can already reach every other service, regardless of any networks: assignment"},
			},
		},
		{
			name: "per-service networks: warns without a top-level networks: block",
			yaml: `
services:
  web:
    image: nginx:latest
    networks: [frontend]
`,
			wants: []Notice{
				{Level: NoticeLevelWarning, Message: "networks: is declared but not enforced; every service in this app shares one network and can already reach every other service, regardless of any networks: assignment"},
			},
		},
		{
			name: "restart and networks together, sorted by service name",
			yaml: `
services:
  web:
    image: nginx:latest
    restart: on-failure:3
    networks: [frontend]
  worker:
    image: worker:latest
    restart: unless-stopped
`,
			wants: []Notice{
				{Level: NoticeLevelNote, Message: `service "web": restart: "on-failure:3" is parsed but not applied; Levelrail's reconciler keeps containers running, not Docker's native restart policy`},
				{Level: NoticeLevelNote, Message: `service "worker": restart: "unless-stopped" is parsed but not applied; Levelrail's reconciler keeps containers running, not Docker's native restart policy`},
				{Level: NoticeLevelWarning, Message: "networks: is declared but not enforced; every service in this app shares one network and can already reach every other service, regardless of any networks: assignment"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got := f.Notices()
			if len(got) != len(tt.wants) {
				t.Fatalf("Notices() = %+v, want %+v", got, tt.wants)
			}
			for i := range got {
				if got[i] != tt.wants[i] {
					t.Errorf("Notices()[%d] = %+v, want %+v", i, got[i], tt.wants[i])
				}
			}
		})
	}
}

func TestParse_NetworksAndRestart(t *testing.T) {
	data := []byte(`
services:
  web:
    image: nginx:latest
    restart: unless-stopped
    networks:
      frontend:
        aliases: [web]
  db:
    image: postgres:16
    restart: "no"
    networks: [backend]
networks:
  frontend:
    driver: bridge
  backend: {}
`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(f.Networks) != 2 || f.Networks[0] != "backend" || f.Networks[1] != "frontend" {
		t.Errorf("f.Networks = %v, want [backend frontend]", f.Networks)
	}

	web := f.Services["web"]
	if web.Restart != "unless-stopped" {
		t.Errorf("web.Restart = %q, want unless-stopped", web.Restart)
	}
	if len(web.Networks) != 1 || web.Networks[0] != "frontend" {
		t.Errorf("web.Networks = %v, want [frontend]", web.Networks)
	}

	db := f.Services["db"]
	if db.Restart != "no" {
		t.Errorf("db.Restart = %q, want no", db.Restart)
	}
	if len(db.Networks) != 1 || db.Networks[0] != "backend" {
		t.Errorf("db.Networks = %v, want [backend]", db.Networks)
	}
}

func TestValidate_RestartAndNetworksDoNotFailValidation(t *testing.T) {
	data := []byte(`
services:
  web:
    image: nginx:latest
    restart: always
    networks: [frontend]
networks:
  frontend: {}
`)
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil: restart/networks are notices, not validation failures", err)
	}
}
