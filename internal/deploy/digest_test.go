package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeResolver struct {
	res   docker.ResolvedImage
	err   error
	calls int
	fresh bool
}

func (f *fakeResolver) ResolveImage(_ context.Context, ref string, _ *docker.RegistryAuth, requireFresh bool) (docker.ResolvedImage, error) {
	f.calls++
	f.fresh = requireFresh
	if f.err != nil {
		return docker.ResolvedImage{}, f.err
	}
	out := f.res
	if out.Ref == "" {
		out.Ref = docker.PinImageRef(ref, out.Digest)
	}
	return out, nil
}

type fakeInspector struct{ id string }

func (f fakeInspector) InspectImageID(context.Context, string) (string, error) { return f.id, nil }

type fakeAttemptRecorder struct{ digest, reason, id string }

func (f *fakeAttemptRecorder) SetDeployAttemptDigest(_ context.Context, id, digest, reason string) error {
	f.id, f.digest, f.reason = id, digest, reason
	return nil
}

func TestResolveImageMapsSources(t *testing.T) {
	regErr := errors.New("dial tcp: registry down")
	tests := []struct {
		name       string
		image      string
		resolver   *fakeResolver
		wantImage  string
		wantDigest string
		wantReason string
		wantCalls  int
	}{
		{name: "no resolver keeps the tag", image: "nginx:latest", wantImage: "nginx:latest"},
		{name: "pinned input never asks the registry", image: "nginx:1@sha256:aa", resolver: &fakeResolver{}, wantImage: "nginx:1@sha256:aa", wantDigest: "sha256:aa", wantReason: store.DigestReasonPinned},
		{name: "registry digest pins the tag", image: "nginx:latest", resolver: &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:new", Source: docker.ImageSourceRegistry}}, wantImage: "nginx:latest@sha256:new", wantDigest: "sha256:new", wantReason: store.DigestReasonResolved, wantCalls: 1},
		{name: "registry down falls back to cache and says so", image: "nginx:latest", resolver: &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:old", Source: docker.ImageSourceCache, RegistryErr: regErr}}, wantImage: "nginx:latest@sha256:old", wantDigest: "sha256:old", wantReason: store.DigestReasonPullFailedUsingCached, wantCalls: 1},
		{name: "nothing known stays unpinned", image: "nginx:latest", resolver: &fakeResolver{res: docker.ResolvedImage{Ref: "nginx:latest", Source: docker.ImageSourceNone, RegistryErr: regErr}}, wantImage: "nginx:latest", wantReason: store.DigestReasonUnresolved, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r docker.ImageResolver
			if tt.resolver != nil {
				r = tt.resolver
			}
			got, err := ResolveImage(context.Background(), r, tt.image, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			if got.Image != tt.wantImage || got.Digest != tt.wantDigest || got.Reason != tt.wantReason {
				t.Fatalf("got %+v", got)
			}
			if tt.resolver != nil && tt.resolver.calls != tt.wantCalls {
				t.Fatalf("resolver calls = %d, want %d", tt.resolver.calls, tt.wantCalls)
			}
		})
	}
}

func TestImageResolutionApplyClearsStaleLocalID(t *testing.T) {
	svc := store.DesiredService{Image: "web:abc", ImageID: "sha256:build", ImageIDRef: "web:abc"}
	ImageResolution{Image: "nginx:1@sha256:x", Digest: "sha256:x"}.apply(&svc)
	if svc.ImageID != "" || svc.LocalImageID() != "" {
		t.Fatalf("a registry deploy must drop the old build's image id, got %+v", svc)
	}
	ImageResolution{Image: "myapp:dev", LocalID: "sha256:loc"}.apply(&svc)
	if svc.LocalImageID() != "sha256:loc" {
		t.Fatalf("local-only image id = %q", svc.LocalImageID())
	}
}

func TestDeployImagePinsDigestAndRecordsAttempt(t *testing.T) {
	st := &fakeServiceStore{}
	rec := &fakeAttemptRecorder{}
	res := &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:new", Source: docker.ImageSourceRegistry}}
	p := New(&fakeBuilder{}, st, WithImageResolver(res), WithAttemptRecorder(rec))
	img, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", AttemptID: "dep_1", RequireFreshImage: true,
		Service: spec.Service{Port: 80, Build: spec.Build{Type: spec.BuildImage, Image: "nginx:latest"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if img != "nginx:latest@sha256:new" || st.saved.Image != img {
		t.Fatalf("returned %q, saved %q", img, st.saved.Image)
	}
	if !res.fresh {
		t.Fatal("RequireFreshImage did not reach the resolver")
	}
	if rec.id != "dep_1" || rec.digest != "sha256:new" || rec.reason != store.DigestReasonResolved {
		t.Fatalf("attempt digest = %+v", rec)
	}
}

func TestDeployImageRegistryDownWithRequireFreshFails(t *testing.T) {
	st := &fakeServiceStore{}
	p := New(&fakeBuilder{}, st, WithImageResolver(&fakeResolver{err: errors.New("registry down")}))
	_, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", RequireFreshImage: true,
		Service: spec.Service{Port: 80, Build: spec.Build{Type: spec.BuildImage, Image: "nginx:latest"}},
	}, nil)
	if err == nil || st.saveCalls != 0 {
		t.Fatalf("err = %v, saves = %d; a failed fresh resolve must not write desired state", err, st.saveCalls)
	}
}

func TestBuildDeployRecordsLocalImageID(t *testing.T) {
	st := &fakeServiceStore{}
	b := &fakeBuilder{result: &build.Result{Tag: "web:abc", Duration: time.Second}}
	p := New(b, st, WithImageInspector(fakeInspector{id: "sha256:built"}))
	if _, err := p.Deploy(context.Background(), Request{
		ServiceName: "web", CommitSHA: "abc", ImageRepo: "web", SourceDir: t.TempDir(),
		Service: spec.Service{Port: 80, Build: spec.Build{Type: spec.BuildDockerfile}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if st.saved.Image != "web:abc" || st.saved.LocalImageID() != "sha256:built" {
		t.Fatalf("saved %+v", st.saved)
	}
}

func TestImageTriggerPinsAndRecordsDigest(t *testing.T) {
	st := &fakeImageDeployStore{}
	res := &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:new", Source: docker.ImageSourceRegistry}}
	got, err := ImageTrigger{Store: st, Resolver: res}.Deploy(context.Background(), store.DesiredService{Name: "web", Image: "nginx:latest@sha256:old"}, "nginx:latest")
	if err != nil {
		t.Fatal(err)
	}
	if got.Image != "nginx:latest@sha256:new" {
		t.Fatalf("image = %q", got.Image)
	}
	if len(st.attempts) != 1 || st.attempts[0].ImageDigest != "sha256:new" || st.attempts[0].Image != got.Image {
		t.Fatalf("attempts = %+v", st.attempts)
	}
}

func TestImageTriggerRollbackToPinnedImageKeepsDigest(t *testing.T) {
	st := &fakeImageDeployStore{}
	res := &fakeResolver{}
	got, err := ImageTrigger{Store: st, Resolver: res}.Deploy(context.Background(), store.DesiredService{Name: "web", Image: "nginx:latest@sha256:new"}, "nginx:latest@sha256:old")
	if err != nil {
		t.Fatal(err)
	}
	if got.Image != "nginx:latest@sha256:old" || res.calls != 0 {
		t.Fatalf("image %q, resolver calls %d", got.Image, res.calls)
	}
	if st.attempts[0].DigestReason != store.DigestReasonPinned {
		t.Fatalf("reason = %q", st.attempts[0].DigestReason)
	}
}
