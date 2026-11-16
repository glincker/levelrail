package k8sconn

import (
	"errors"
	"strings"
	"testing"
)

func TestImageRegistryHost(t *testing.T) {
	for image, want := range map[string]string{
		"nginx:1":                "docker.io",
		"acme/api:1":             "docker.io",
		"ghcr.io/acme/api:1":     "ghcr.io",
		"reg.example.com:5000/a": "reg.example.com:5000",
		"GHCR.io/a":              "ghcr.io",
		"localhost:5000/a":       "localhost:5000",
	} {
		if got := ImageRegistryHost(image); got != want {
			t.Errorf("%s = %q, want %q", image, got, want)
		}
	}
	if NormalizeRegistryHost("https://Index.Docker.io/v1/") != "docker.io" || NormalizeRegistryHost("bad host") != "" {
		t.Fatal("normalize wrong")
	}
}

func TestBuildPullSecretMismatchAndRender(t *testing.T) {
	spec := AppSpec{Name: "api", Namespace: "ns", Image: "ghcr.io/a/b:1", InstanceID: "i1", PullSecretName: "api-registry"}
	if _, err := BuildPullSecret(spec, "quay.io", "u", "p"); !errors.Is(err, ErrRegistryMismatch) {
		t.Fatalf("err = %v", err)
	}
	obj, err := BuildPullSecret(spec, "ghcr.io", "u", "p")
	if err != nil || obj["type"] != SecretTypeDockerConfig {
		t.Fatalf("secret = %v %v", obj, err)
	}
	spec.Resources = Resources{"1m", "1Mi", "2m", "2Mi"}
	spec.Port, spec.Replicas = 80, 1
	r, err := Render(spec)
	if err != nil {
		t.Fatal(err)
	}
	r.Secret = obj
	y, _ := r.YAML()
	if !strings.Contains(string(y), "imagePullSecrets") || strings.Contains(string(y), "dockerconfigjson") {
		t.Fatalf("yaml must reference the secret and never print it: %s", y)
	}
}
