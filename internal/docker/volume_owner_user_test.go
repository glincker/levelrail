package docker

import (
	"context"
	"testing"
)

// A nil engine client proves the root spec user short-circuits before any image inspect.
func TestChownFreshVolumes_SpecRootUserSkipsImageLookup(t *testing.T) {
	c := &Client{}
	vols := []VolumeMount{{Name: "data", ContainerPath: "/data"}}
	for _, u := range []string{"root", "0", "0:0"} {
		if err := c.chownFreshVolumes(context.Background(), "cid", "img:1", u, vols); err != nil {
			t.Fatalf("chownFreshVolumes(user %q) error = %v", u, err)
		}
	}
}
