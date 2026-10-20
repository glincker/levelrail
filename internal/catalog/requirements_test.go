package catalog

import "testing"

func TestComputeRequirements(t *testing.T) {
	tests := []struct {
		name      string
		compose   string
		level     RequirementsLevel
		wantCodes []string
		wantProt  []string
		noProt    []string
	}{
		{
			name: "pinned with volume and healthcheck",
			compose: `services:
  web:
    image: nginx:1.27
    volumes:
      - data:/data
    healthcheck:
      test: ["CMD", "true"]
`,
			level:     LevelStandard,
			wantCodes: []string{FindingPersistentData},
			wantProt:  []string{ProtectionBackups, ProtectionHealth, ProtectionRollback},
			noProt:    []string{ProtectionMasking},
		},
		{
			name: "latest tag and host mount",
			compose: `services:
  web:
    image: nginx:latest
    volumes:
      - /srv/x:/x
`,
			level:     LevelReview,
			wantCodes: []string{FindingUnpinnedImage, FindingHostMount, FindingNoHealthcheck},
			noProt:    []string{ProtectionBackups, ProtectionHealth},
		},
		{
			name: "docker socket is elevated",
			compose: `services:
  agent:
    image: foo/agent:1
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
`,
			level:     LevelElevated,
			wantCodes: []string{FindingDockerSocket},
		},
		{
			name: "privileged host net caps",
			compose: `services:
  vpn:
    image: foo/vpn:2
    privileged: true
    network_mode: host
    cap_add: [NET_ADMIN]
`,
			level:     LevelElevated,
			wantCodes: []string{FindingPrivileged, FindingHostNetwork, FindingCapAdd},
		},
		{
			name: "generated secret and published port",
			compose: `services:
  db:
    image: postgres:16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_PASSWORD: ${SERVICE_PASSWORD_DB}
`,
			level:     LevelStandard,
			wantCodes: []string{FindingPublishedPorts, FindingGeneratedSecrets},
			wantProt:  []string{ProtectionMasking},
		},
		{
			name:      "unparseable",
			compose:   "services: [",
			level:     LevelReview,
			wantCodes: []string{FindingUnparsed},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeRequirements(tc.compose)
			if got.Level != tc.level {
				t.Fatalf("level = %s, want %s (%+v)", got.Level, tc.level, got.Findings)
			}
			codes := map[string]bool{}
			for _, f := range got.Findings {
				codes[f.Code] = true
			}
			for _, c := range tc.wantCodes {
				if !codes[c] {
					t.Errorf("missing finding %s in %+v", c, got.Findings)
				}
			}
			prot := map[string]bool{}
			for _, p := range got.Protections {
				prot[p.Code] = true
			}
			for _, c := range tc.wantProt {
				if !prot[c] {
					t.Errorf("missing protection %s", c)
				}
			}
			for _, c := range tc.noProt {
				if prot[c] {
					t.Errorf("unexpected protection %s", c)
				}
			}
		})
	}
}

func TestComputeRequirementsWholeCatalog(t *testing.T) {
	for _, tpl := range Templates {
		if r := ComputeRequirements(tpl.Compose); r.Level == "" || r.Findings == nil || r.Protections == nil {
			t.Fatalf("%s: incomplete result", tpl.ID)
		}
	}
}
