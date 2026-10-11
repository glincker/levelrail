package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// dindNode is one Docker-in-Docker daemon standing in for a separate host.
// Its IP on the shared test network plays the node's mesh address.
type dindNode struct {
	name   string
	host   string
	meshIP string
}

const (
	dindImage   = "docker:27-dind"
	dindNetwork = "lr-multinode-live"
)

func liveHostDocker(t *testing.T) *dockerclient.Client {
	t.Helper()
	if os.Getenv("LEVELRAIL_LIVE_MULTINODE") == "" {
		t.Skip("set LEVELRAIL_LIVE_MULTINODE=1 to run the two-daemon multi-node live test (needs privileged containers)")
	}
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func ensureHostImage(ctx context.Context, t *testing.T, cli *dockerclient.Client, ref string) {
	t.Helper()
	if _, err := cli.ImageInspect(ctx, ref); err == nil {
		return
	}
	rc, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		t.Fatalf("pull %s: %v", ref, err)
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}

// startDindNodes starts n privileged daemons on one network and loads images into each.
func startDindNodes(ctx context.Context, t *testing.T, cli *dockerclient.Client, n int, images ...string) []dindNode {
	t.Helper()
	ensureHostImage(ctx, t, cli, dindImage)
	for _, ref := range images {
		ensureHostImage(ctx, t, cli, ref)
	}
	for i := 0; i < n; i++ {
		_ = cli.ContainerRemove(ctx, fmt.Sprintf("lr-multinode-live-%d", i), container.RemoveOptions{Force: true, RemoveVolumes: true})
	}
	_ = cli.NetworkRemove(ctx, dindNetwork)
	if _, err := cli.NetworkCreate(ctx, dindNetwork, network.CreateOptions{}); err != nil {
		t.Fatalf("create network: %v", err)
	}
	t.Cleanup(func() { _ = cli.NetworkRemove(context.Background(), dindNetwork) })

	nodes := make([]dindNode, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("lr-multinode-live-%d", i)
		_ = cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true, RemoveVolumes: true})
		created, err := cli.ContainerCreate(ctx,
			&container.Config{Image: dindImage, Env: []string{"DOCKER_TLS_CERTDIR="}, Cmd: []string{"--host=tcp://0.0.0.0:2375", "--tls=false"},
				ExposedPorts: nat.PortSet{"2375/tcp": {}}},
			&container.HostConfig{Privileged: true, PortBindings: nat.PortMap{"2375/tcp": {{HostIP: "127.0.0.1", HostPort: "0"}}}},
			&network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{dindNetwork: {}}}, nil, name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() {
			_ = cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		})
		if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
		info, err := cli.ContainerInspect(ctx, created.ID)
		if err != nil {
			t.Fatalf("inspect %s: %v", name, err)
		}
		bindings := info.NetworkSettings.Ports["2375/tcp"]
		if len(bindings) == 0 {
			t.Fatalf("%s: no published daemon port", name)
		}
		node := dindNode{name: name, host: "tcp://127.0.0.1:" + bindings[0].HostPort, meshIP: info.NetworkSettings.Networks[dindNetwork].IPAddress}
		waitDind(ctx, t, node)
		loadImages(ctx, t, cli, node, images)
		nodes = append(nodes, node)
	}
	return nodes
}

func waitDind(ctx context.Context, t *testing.T, node dindNode) {
	t.Helper()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.WithHost(node.host), dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("client for %s: %v", node.name, err)
	}
	defer func() { _ = cli.Close() }()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err = cli.Ping(pctx)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s never answered: %v", node.name, err)
}

func loadImages(ctx context.Context, t *testing.T, host *dockerclient.Client, node dindNode, images []string) {
	t.Helper()
	if len(images) == 0 {
		return
	}
	rc, err := host.ImageSave(ctx, images)
	if err != nil {
		t.Fatalf("save images: %v", err)
	}
	defer func() { _ = rc.Close() }()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.WithHost(node.host), dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("client for %s: %v", node.name, err)
	}
	defer func() { _ = cli.Close() }()
	resp, err := cli.ImageLoad(ctx, rc)
	if err != nil {
		t.Fatalf("load images into %s: %v", node.name, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if strings.Contains(string(body), `"error"`) {
		t.Fatalf("load images into %s: %s", node.name, body)
	}
}
