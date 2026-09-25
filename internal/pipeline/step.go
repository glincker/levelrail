package pipeline

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

var (
	secretRefRe = regexp.MustCompile(`secrets\.([A-Za-z_][A-Za-z0-9_]*)`)
	outputRe    = regexp.MustCompile(`^::set-output name=([A-Za-z0-9_.-]+)::(.*)$`)
)

// execStep runs one step and returns its exit code (when it ran a command)
// and an error when it failed.
func (jr *jobRun) execStep(ctx context.Context, k int, p plannedStep, step Step) (*int, error) {
	switch p.Kind {
	case KindSetup:
		return nil, jr.setup(ctx, k)
	case KindRun, KindTest, KindArtifactUpload, KindArtifactDownload:
		return jr.execContainerStep(ctx, k, p, step)
	}
	return nil, jr.execControlStep(ctx, k, p, step)
}

// stepScope resolves the scope for a step, fetching every secret it
// references and registering the values for log masking.
func (jr *jobRun) stepScope(ctx context.Context, step Step, texts ...string) (Scope, error) {
	sc := jobScope(jr.run, jr.def, jr.row, jr.all)
	jr.addOutputScope(&sc)
	sc.Success = true
	names := map[string]bool{}
	for _, n := range step.Secrets {
		names[n] = true
	}
	all := append([]string{}, texts...)
	all = append(all, mapText(step.Env)...)
	all = append(all, mapText(step.With)...)
	all = append(all, mapText(jr.jd.Env)...)
	all = append(all, mapText(jr.def.Env)...)
	for _, t := range all {
		for _, m := range secretRefRe.FindAllStringSubmatch(t, -1) {
			names[m[1]] = true
		}
	}
	for n := range names {
		if jr.e.cfg.Secrets == nil {
			return sc, errors.New("secrets are not configured on this control plane")
		}
		v, err := jr.e.cfg.Secrets.Resolve(ctx, jr.run.AppName, n)
		if err != nil {
			return sc, fmt.Errorf("secret %q: %w", n, err)
		}
		sc.Vars["secrets."+n] = v
		jr.mask.add(v)
	}
	return sc, nil
}

func mapText(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func (jr *jobRun) stepEnv(step Step, sc Scope) (map[string]string, error) {
	env := map[string]string{
		"CI": "true", "PIPELINE_RUN_ID": jr.run.ID, "PIPELINE_RUN_NUMBER": fmt.Sprint(jr.run.Number), "PIPELINE_JOB": jr.row.Key,
		"PIPELINE_SHA": jr.run.CommitSHA, "PIPELINE_REF": jr.run.Ref, "PIPELINE_APP": jr.run.AppName,
	}
	for _, layer := range []map[string]string{jr.def.Env, jr.jd.Env, step.Env} {
		for k, v := range layer {
			iv, err := Interpolate(v, sc)
			if err != nil {
				return nil, fmt.Errorf("env %s: %w", k, err)
			}
			env[k] = iv
		}
	}
	for _, n := range step.Secrets {
		env[n] = sc.Vars["secrets."+n]
	}
	return env, nil
}

func (jr *jobRun) execContainerStep(ctx context.Context, k int, p plannedStep, step Step) (*int, error) {
	script, err := jr.stepScript(p.Kind, step)
	if err != nil {
		return nil, err
	}
	sc, err := jr.stepScope(ctx, step, script)
	if err != nil {
		return nil, err
	}
	body, err := Interpolate(script, sc)
	if err != nil {
		return nil, err
	}
	env, err := jr.stepEnv(step, sc)
	if err != nil {
		return nil, err
	}
	cid, err := jr.ensureContainer(ctx)
	if err != nil {
		return nil, err
	}
	return jr.exec(ctx, k, cid, buildScript(env, body))
}

func (jr *jobRun) stepScript(kind string, step Step) (string, error) {
	switch kind {
	case KindTest:
		return step.With["command"], nil
	case KindArtifactUpload:
		name, path := step.With["name"], step.With["path"]
		if !safeArtifactName(name) {
			return "", fmt.Errorf("invalid artifact name %q", name)
		}
		return fmt.Sprintf("mkdir -p /artifacts/%s && cp -a %s /artifacts/%s/", name, shellQuote(path), name), nil
	case KindArtifactDownload:
		name := step.With["name"]
		if !safeArtifactName(name) {
			return "", fmt.Errorf("invalid artifact name %q", name)
		}
		dest := firstNonEmpty(step.With["path"], ".")
		return fmt.Sprintf("test -d /artifacts/%s || { echo 'artifact %s not found' >&2; exit 1; }\nmkdir -p %s && cp -a /artifacts/%s/. %s/", name, name, shellQuote(dest), name, shellQuote(dest)), nil
	}
	return step.Run, nil
}

var artifactNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func safeArtifactName(n string) bool { return artifactNameRe.MatchString(n) }

func buildScript(env map[string]string, body string) string {
	var sb strings.Builder
	sb.WriteString("exec 2>&1\nset -e\ncd /workspace\n")
	for _, k := range sortedKeys(env) {
		if envKeyRe.MatchString(k) {
			fmt.Fprintf(&sb, "export %s=%s\n", k, shellQuote(env[k]))
		}
	}
	sb.WriteString(body)
	sb.WriteString("\n")
	return sb.String()
}

// exec streams a script through `sh -s` in the container so secrets never
// appear in the exec's argv.
func (jr *jobRun) exec(ctx context.Context, k int, cid, script string) (*int, error) {
	rc, err := jr.rt.ExecWithInput(ctx, cid, []string{"sh", "-s"}, strings.NewReader(script))
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = rc.Close() })
	defer func() {
		stop()
		_ = rc.Close()
	}()
	readErr := readLines(rc, func(line string) {
		if m := outputRe.FindStringSubmatch(line); m != nil {
			jr.setOutput(m[1], m[2])
		}
		jr.sink.Line(k, "stdout", line)
	})
	if readErr == nil {
		zero := 0
		return &zero, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var ee *docker.ExecExitError
	if errors.As(readErr, &ee) {
		code := ee.ExitCode
		return &code, fmt.Errorf("exited with code %d", code)
	}
	return nil, fmt.Errorf("read output: %w", readErr)
}

func (jr *jobRun) setOutput(name, value string) {
	jr.mu.Lock()
	jr.outputs[name] = jr.mask.mask(value)
	jr.mu.Unlock()
}

// setup checks out the repository into the job workspace and starts the job
// container. Re-running it on resume only recreates the container.
func (jr *jobRun) setup(ctx context.Context, k int) error {
	if _, err := jr.ensureContainer(ctx); err != nil {
		return err
	}
	if jr.jd.Checkout != nil && !*jr.jd.Checkout {
		jr.sink.Line(k, "stdout", "checkout disabled")
		return nil
	}
	if jr.e.cfg.Source == nil {
		jr.sink.Line(k, "stdout", "no repository source configured, skipping checkout")
		return nil
	}
	url, token, err := jr.e.cfg.Source.RepoInfo(ctx, jr.run.AppName)
	if errors.Is(err, ErrNoRepo) {
		jr.sink.Line(k, "stdout", "app has no connected repository, skipping checkout")
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	return jr.checkout(ctx, k, url, token)
}

func (jr *jobRun) checkout(ctx context.Context, k int, url, token string) error {
	e := jr.e
	ref := firstNonEmpty(jr.run.CommitSHA, jr.run.Ref, "HEAD")
	auth := ""
	if token != "" {
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		jr.mask.add(token)
		jr.mask.add(basic)
		auth = "-c http.extraHeader=" + shellQuote("Authorization: Basic "+basic)
	}
	name := e.jobPrefix(jr.run.ID, jr.row.ID) + "-git"
	id, err := jr.rt.Create(ctx, docker.ContainerSpec{
		Name: name, Image: e.cfg.GitImage, Entrypoint: keepAliveEntrypoint,
		Volumes: []docker.VolumeMount{{Name: e.workspaceVolume(jr.run.ID, jr.row.ID), ContainerPath: "/workspace"}},
	})
	if err != nil {
		return fmt.Errorf("create checkout container: %w", err)
	}
	defer func() { _ = jr.rt.Remove(context.WithoutCancel(ctx), id, true) }()
	if err := jr.rt.Start(ctx, id); err != nil {
		return fmt.Errorf("start checkout container: %w", err)
	}
	target := "FETCH_HEAD"
	if jr.run.CommitSHA != "" {
		target = jr.run.CommitSHA
	}
	body := fmt.Sprintf("git init -q .\ngit remote add origin %s\ngit %s fetch -q --depth 1 origin %s || git %s fetch -q origin\ngit checkout -q --force %s\n",
		shellQuote(url), auth, shellQuote(ref), auth, shellQuote(target))
	jr.sink.Line(k, "stdout", fmt.Sprintf("checking out %s", ref))
	if _, err := jr.exec(ctx, k, id, buildScript(nil, body)); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}
	return nil
}

func (jr *jobRun) execControlStep(ctx context.Context, k int, p plannedStep, step Step) error {
	act := jr.e.cfg.Actions
	if act == nil {
		return errors.New("build and deploy actions are not configured on this control plane")
	}
	sc, err := jr.stepScope(ctx, step)
	if err != nil {
		return err
	}
	with := mapValues(step.With, func(v string) string {
		out, ierr := Interpolate(v, sc)
		if ierr != nil && err == nil {
			err = ierr
		}
		return out
	})
	if err != nil {
		return err
	}
	logf := func(line string) { jr.sink.Line(k, "stdout", line) }
	app := jr.run.AppName

	switch p.Kind {
	case KindBuild:
		tag := firstNonEmpty(with["tag"], shortSHA(jr.run.CommitSHA), "run"+fmt.Sprint(jr.run.Number))
		image, err := act.Build(ctx, BuildRequest{
			App: app, Ref: jr.run.Ref, SHA: jr.run.CommitSHA, Context: firstNonEmpty(with["context"], "."), Dockerfile: with["dockerfile"],
			Image: firstNonEmpty(with["image"], jr.e.cfg.NamePrefix+"/"+app), Tag: tag,
		}, logf)
		if err != nil {
			return fmt.Errorf("build: %w", err)
		}
		jr.setOutput("image", image)
		if step.ID != "" {
			jr.setOutput("steps."+step.ID+".image", image)
		}
		logf("built " + image)
	case KindDeploy:
		image := firstNonEmpty(with["image"], jr.upstreamImage())
		if image == "" {
			return errors.New("deploy needs with.image or a needed job that outputs an image")
		}
		wait := 5 * time.Minute
		if v := with["wait"]; v != "" {
			d, perr := time.ParseDuration(v)
			if perr != nil {
				return fmt.Errorf("with.wait: %w", perr)
			}
			wait = d
		}
		svc := firstNonEmpty(with["service"], app)
		if err := act.Deploy(ctx, DeployRequest{Service: svc, Image: image, Strategy: with["strategy"], Wait: wait}, logf); err != nil {
			return fmt.Errorf("deploy %s: %w", svc, err)
		}
		jr.setOutput("deployed_image", image)
	case KindPromote:
		image, err := act.Promote(ctx, with["from"], with["to"], logf)
		if err != nil {
			return fmt.Errorf("promote %s to %s: %w", with["from"], with["to"], err)
		}
		jr.setOutput("image", image)
	case KindRollback:
		svc := firstNonEmpty(with["service"], app)
		image, err := act.Rollback(ctx, svc, logf)
		if err != nil {
			return fmt.Errorf("rollback %s: %w", svc, err)
		}
		jr.setOutput("image", image)
	case KindNotify:
		jr.mu.Lock()
		ok := !jr.failed
		jr.mu.Unlock()
		switch with["on"] {
		case "failure":
			if ok {
				return nil
			}
		case "success":
			if !ok {
				return nil
			}
		}
		if err := act.Notify(ctx, firstNonEmpty(with["app"], app), ok, with["message"]); err != nil {
			return fmt.Errorf("notify: %w", err)
		}
		logf("notification sent")
	default:
		return fmt.Errorf("unsupported step kind %q", p.Kind)
	}
	return nil
}

// upstreamImage finds the image output of the nearest needed job.
func (jr *jobRun) upstreamImage() string {
	jr.mu.Lock()
	if v := jr.outputs["image"]; v != "" {
		jr.mu.Unlock()
		return v
	}
	jr.mu.Unlock()
	sc := jobScope(jr.run, jr.def, jr.row, jr.all)
	var needs []string
	for k := range sc.Vars {
		if n, ok := strings.CutSuffix(k, ".outputs.image"); ok && strings.HasPrefix(n, "needs.") {
			needs = append(needs, k)
		}
	}
	if len(needs) == 0 {
		return ""
	}
	return sc.Vars[sortedFirst(needs)]
}

func sortedFirst(s []string) string {
	best := s[0]
	for _, v := range s[1:] {
		if v < best {
			best = v
		}
	}
	return best
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
