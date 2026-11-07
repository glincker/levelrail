package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envPreviewMigrateTimeout     = "APP_PREVIEW_MIGRATE_TIMEOUT"
	defaultPreviewMigrateTimeout = 10 * time.Minute
	envPreviewMigrateWait        = "APP_PREVIEW_MIGRATE_WAIT"
	defaultPreviewMigrateWait    = 2 * time.Minute
	maxPreviewMigrateSeconds     = 24 * 3600
	maxPreviewMigrateCommand     = 4096
	maxPreviewImageTemplate      = 512
	// previewMigrateReasonCap keeps the failure reason readable in a PR comment.
	previewMigrateReasonCap = 3000
	previewMigratePoll      = 2 * time.Second
)

var previewImagePlaceholder = regexp.MustCompile(`\{(number|sha|short_sha|branch)\}`)

// previewMigrateTimeout is the app's own bound, else the platform default.
func previewMigrateTimeout(s store.PreviewAppSettings) time.Duration {
	if s.MigrateTimeoutSeconds > 0 {
		return time.Duration(s.MigrateTimeoutSeconds) * time.Second
	}
	return envDuration(envPreviewMigrateTimeout, defaultPreviewMigrateTimeout)
}

// applyPreviewMigrateFields validates the migrate and image tag fields of a
// policy update, returning a client-facing message for the first bad value.
func applyPreviewMigrateFields(s *store.PreviewAppSettings, req setPreviewPolicyRequest) string {
	if req.MigrateCommand != nil {
		c := strings.TrimSpace(*req.MigrateCommand)
		if len(c) > maxPreviewMigrateCommand {
			return "migrate_command must be at most " + strconv.Itoa(maxPreviewMigrateCommand) + " characters"
		}
		s.MigrateCommand = c
	}
	if req.MigrateTimeoutSeconds != nil {
		if *req.MigrateTimeoutSeconds < 0 || *req.MigrateTimeoutSeconds > maxPreviewMigrateSeconds {
			return "migrate_timeout_seconds must be between 0 and " + strconv.Itoa(maxPreviewMigrateSeconds) + " (0 uses the platform default)"
		}
		s.MigrateTimeoutSeconds = *req.MigrateTimeoutSeconds
	}
	if req.ImageTagTemplate != nil {
		t := strings.TrimSpace(*req.ImageTagTemplate)
		if t != "" {
			if len(t) > maxPreviewImageTemplate || strings.ContainsAny(t, " \t\r\n") {
				return "image_tag_template must be an image reference without spaces, at most " + strconv.Itoa(maxPreviewImageTemplate) + " characters"
			}
			if !previewImagePlaceholder.MatchString(t) {
				return "image_tag_template must contain {number}, {sha}, {short_sha} or {branch} so each pull request gets its own tag"
			}
		}
		s.ImageTagTemplate = t
	}
	return ""
}

// previewImageTag expands template for one pull request, or returns "" when
// no template is set. The branch is reduced to characters valid in a tag.
func previewImageTag(template string, number int, sha, branch string) string {
	if template == "" {
		return ""
	}
	short := sha
	if len(short) > 7 {
		short = short[:7]
	}
	return strings.NewReplacer(
		"{number}", strconv.Itoa(number), "{sha}", sha, "{short_sha}", short, "{branch}", sanitizeImageTagPart(branch),
	).Replace(template)
}

func sanitizeImageTagPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// runPreviewMigrate runs the app's migrate command once in the preview's
// container, which already carries the preview env and database. It returns a
// note on success and an error carrying the masked output on failure; no
// command configured is a quiet no-op.
func (rt *Router) runPreviewMigrate(ctx context.Context, previewName string, s store.PreviewAppSettings) (string, error) {
	if s.MigrateCommand == "" {
		return "", nil
	}
	if rt.execRuntime == nil {
		return "", errors.New("migrate: this control plane cannot run commands in containers")
	}
	svc, err := rt.apps.GetDesiredService(ctx, previewName)
	if err != nil {
		return "", fmt.Errorf("migrate: load preview service: %w", err)
	}
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return "", fmt.Errorf("migrate: preview node is not reachable: %w", err)
	}
	target := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)

	containerID, err := waitPreviewContainer(ctx, runtime, target, envDuration(envPreviewMigrateWait, defaultPreviewMigrateWait))
	if err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, previewMigrateTimeout(s))
	defer cancel()
	rc, err := runtime.Exec(execCtx, containerID, []string{"sh", "-c", s.MigrateCommand})
	if err != nil {
		return "", fmt.Errorf("migrate: start command: %w", err)
	}
	defer func() { _ = rc.Close() }()

	out := &cappedWriter{limit: execMaxOutputBytes}
	_, copyErr := io.Copy(out, rc)
	text := out.buf.String()
	if copyErr == nil {
		return "migration ran: " + s.MigrateCommand, nil
	}
	var execErr *docker.ExecExitError
	switch {
	case errors.As(copyErr, &execErr):
		text += "\n" + execErr.Stderr
		return "", fmt.Errorf("migration failed (exit %d): %s", execErr.ExitCode, tailMasked(text))
	case errors.Is(execCtx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("migration timed out after %s: %s", previewMigrateTimeout(s), tailMasked(text))
	default:
		return "", fmt.Errorf("migration output could not be read: %w", copyErr)
	}
}

// waitPreviewContainer polls until the named container runs, since the
// reconciler may still be replacing it after the database was attached.
func waitPreviewContainer(ctx context.Context, rtm docker.Runtime, name string, wait time.Duration) (string, error) {
	deadline := time.Now().Add(wait)
	for {
		inspectCtx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
		state, err := rtm.InspectByName(inspectCtx, name)
		cancel()
		if err == nil && state != nil && state.Running {
			return state.ID, nil
		}
		if !time.Now().Before(deadline) {
			return "", errors.New("the preview container is not running")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(previewMigratePoll):
		}
	}
}

// tailMasked masks secrets, then keeps the end of the output where a failing
// command states its error.
func tailMasked(s string) string {
	s = strings.TrimSpace(secretmask.Mask(s))
	if len(s) > previewMigrateReasonCap {
		s = "..." + s[len(s)-previewMigrateReasonCap:]
	}
	return s
}
