package compose

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/dockerguard"
)

const socketCompose = `services:
  logs:
    image: amir20/dozzle:latest
    ports: ["8080"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
`

func TestDockerSocketNeedsGrant(t *testing.T) {
	f, err := Parse([]byte(socketCompose))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Validate(); err == nil || !strings.Contains(err.Error(), "Docker socket") {
		t.Fatalf("socket without a grant must be refused, got %v", err)
	}

	f.CapGrants = map[string][]string{"logs": {dockerguard.GrantCap(dockerguard.ProfileReadOnly)}}
	svcs, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("granted socket refused: %v", err)
	}
	if len(svcs[0].BindMounts) != 0 {
		t.Fatalf("the real socket must never be stored as a bind: %+v", svcs[0].BindMounts)
	}
	if p, ok := dockerguard.ProfileFromCaps(svcs[0].GrantedCaps); !ok || p != dockerguard.ProfileReadOnly {
		t.Fatalf("grant token missing from granted caps: %v", svcs[0].GrantedCaps)
	}
}

func TestDockerSocketOtherPathsStayRefused(t *testing.T) {
	body := strings.Replace(socketCompose, "/var/run/docker.sock:/var/run/docker.sock", "/var/run/docker.sock:/other.sock", 1)
	f, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	f.CapGrants = map[string][]string{"logs": {dockerguard.GrantCap(dockerguard.ProfileProject)}}
	if err := f.Validate(); err == nil {
		t.Fatal("a grant must not cover a socket mounted at a custom path")
	}
}

func TestDockerSocketRollbackDropsUngranted(t *testing.T) {
	f, err := Parse([]byte(socketCompose))
	if err != nil {
		t.Fatal(err)
	}
	f.DropUngrantedCaps = true
	svcs, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("rollback must not be stranded: %v", err)
	}
	if len(svcs[0].BindMounts) != 0 || len(svcs[0].GrantedCaps) != 0 {
		t.Fatalf("ungranted socket must be dropped: %+v", svcs[0])
	}
}
