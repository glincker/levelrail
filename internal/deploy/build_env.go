package deploy

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

const stepBuildEnv = "build env"

// dockerfilePathOrDefault is the Dockerfile the build will read.
func dockerfilePathOrDefault(buildRoot, dockerfilePath string) string {
	if dockerfilePath != "" {
		return dockerfilePath
	}
	return filepath.Join(buildRoot, "Dockerfile")
}

// dockerfileArgNames lists the names a Dockerfile declares with ARG. An
// unreadable file yields none, so nothing extra is passed.
func dockerfileArgNames(path string) map[string]bool {
	f, err := os.Open(path) //nolint:gosec // path is inside the checkout, validated by the caller
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck // read only
	names := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || !strings.EqualFold(fields[0], "ARG") {
			continue
		}
		for _, f := range fields[1:] {
			names[strings.SplitN(f, "=", 2)[0]] = true
		}
	}
	return names
}

// dockerfileBuildArgs adds public literal env to a Dockerfile build, only for
// names the Dockerfile declares with ARG. Explicit build.args win.
func dockerfileBuildArgs(env map[string]spec.EnvVar, args map[string]string, dockerfile string, prefixes []string) map[string]string {
	declared := dockerfileArgNames(dockerfile)
	out := map[string]string{}
	for k, v := range staticBuildArgs(env, nil, prefixes) {
		if declared[k] {
			out[k] = v
		}
	}
	for k, v := range args {
		out[k] = v
	}
	if len(out) == 0 {
		return args
	}
	return out
}

// emitBuildEnvKeys logs which env names reach the build, never the values,
// so a missing NEXT_PUBLIC_ variable can be diagnosed from the build log.
func emitBuildEnvKeys(progress func(build.ProgressEvent), kind string, env map[string]string, appEnv int) {
	// An app with no env at all has nothing to explain.
	if progress == nil || (len(env) == 0 && appEnv == 0) {
		return
	}
	line := "build env (" + kind + "): none passed to the build; public-prefix literal app env vars are passed, secrets never are"
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		line = "build env (" + kind + "): passing " + strings.Join(keys, ", ")
	}
	progress(build.ProgressEvent{Step: stepBuildEnv, Log: line, Stream: "stdout"})
}
