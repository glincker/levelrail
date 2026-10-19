package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type memNodeFiles struct{ objs map[string][]byte }

func (m *memNodeFiles) Put(_ context.Context, node, path string, r io.Reader) error {
	b, _ := io.ReadAll(r)
	m.objs[node+"|"+path] = b
	return nil
}

func (m *memNodeFiles) Get(_ context.Context, node, path string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.objs[node+"|"+path])), nil
}

func (m *memNodeFiles) Delete(_ context.Context, node, path string) error {
	delete(m.objs, node+"|"+path)
	return nil
}

func (m *memNodeFiles) List(_ context.Context, node, prefix string) ([]RemoteObject, error) {
	var out []RemoteObject
	for k, v := range m.objs {
		if p, ok := strings.CutPrefix(k, node+"|"); ok && strings.HasPrefix(p, prefix) {
			out = append(out, RemoteObject{Key: p, Size: int64(len(v))})
		}
	}
	return out, nil
}

func TestRoutedAgentDestination(t *testing.T) {
	files := &memNodeFiles{objs: map[string][]byte{}}
	r := Routed{Files: files}
	dest := Destination{Provider: "custom", Endpoint: AgentEndpoint("node-1"), Bucket: "backups"}
	ctx := context.Background()

	if err := r.Test(ctx, dest); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if len(files.objs) != 0 {
		t.Fatalf("probe object left behind: %v", files.objs)
	}
	if err := r.Upload(ctx, dest, "db/1.dump", strings.NewReader("dump"), -1); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	objs, err := r.List(ctx, dest, "db/")
	if err != nil || len(objs) != 1 || objs[0].Key != "db/1.dump" || objs[0].Size != 4 {
		t.Fatalf("List = %v, %v", objs, err)
	}
	rc, err := r.Download(ctx, dest, "db/1.dump")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "dump" {
		t.Fatalf("Download = %q", got)
	}
	if err := r.Delete(ctx, dest, "db/1.dump"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestRoutedAgentDestinationWithoutTransport(t *testing.T) {
	dest := Destination{Endpoint: AgentEndpoint("node-1"), Bucket: "backups"}
	err := Routed{}.Upload(context.Background(), dest, "k", strings.NewReader("x"), -1)
	if !errors.Is(err, ErrNodeFilesUnavailable) {
		t.Fatalf("error = %v, want ErrNodeFilesUnavailable", err)
	}
}
