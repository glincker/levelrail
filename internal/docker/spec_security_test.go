package docker

import (
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

func TestBuildHostConfig_SecurityFields(t *testing.T) {
	hc := buildHostConfig(ContainerSpec{
		UsernsMode:  "keep-id",
		SecurityOpt: []string{"label=type:container_runtime_t"},
		Devices:     []string{"/dev/fuse", "/dev/a:/dev/b:r"},
	}, nat.PortMap{})
	if hc.UsernsMode != "keep-id" || !reflect.DeepEqual(hc.SecurityOpt, []string{"label=type:container_runtime_t"}) {
		t.Fatalf("userns/security: %q %v", hc.UsernsMode, hc.SecurityOpt)
	}
	want := []container.DeviceMapping{
		{PathOnHost: "/dev/fuse", PathInContainer: "/dev/fuse", CgroupPermissions: "rwm"},
		{PathOnHost: "/dev/a", PathInContainer: "/dev/b", CgroupPermissions: "r"},
	}
	if !reflect.DeepEqual(hc.Devices, want) {
		t.Fatalf("devices %+v", hc.Devices)
	}
}

func TestBuildHostConfig_SecurityFieldsDefaultUnset(t *testing.T) {
	hc := buildHostConfig(ContainerSpec{}, nat.PortMap{})
	if hc.UsernsMode != "" || len(hc.SecurityOpt) != 0 || len(hc.Devices) != 0 {
		t.Fatalf("unset spec changed host config: %+v", hc)
	}
}
