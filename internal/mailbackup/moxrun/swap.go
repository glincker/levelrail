package moxrun

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// swapScript is the journaled, reversible swap run in the restore helper.
// Every mode works on one volume root ($2). The journal (.levelrail/swap)
// records the phase before each step, so rollback can always restore the
// original entries from the pre-restore dir, whatever step was interrupted.
const swapScript = `set -eu
mode="$1"; R="$2"; id="${3:-}"; pre="${4:-}"; arg="${5:-}"
L="$R/.levelrail"; j="$L/swap"
cd "$R"
mkdir -p "$L"; chmod 700 "$L"
jw() { printf '%s %s %s\n' "$id" "$pre" "$1" > "$j.new"; sync; mv -f "$j.new" "$j"; sync; }
case "$mode" in
swap)
	s="$L/restore-$id/$arg"; p="$L/$pre"
	test -d "$s" || { echo "staged restore $s is missing" >&2; exit 3; }
	[ ! -e "$j" ] || { echo "an earlier restore swap in $R is not finished" >&2; exit 4; }
	[ ! -e "$p" ] || { echo "$p already exists" >&2; exit 5; }
	chown -R "$(ls -nd . | awk '{print $3":"$4}')" "$s"
	mkdir -p "$p"; chmod 700 "$p"
	jw aside
	for f in * .[!.]* ..?*; do { [ -e "$f" ] || [ -L "$f" ]; } || continue; case "$f" in tmp|.levelrail) continue;; esac; mv -- "$f" "$p/"; done
	jw in
	for f in "$s"/* "$s"/.[!.]* "$s"/..?*; do { [ -e "$f" ] || [ -L "$f" ]; } || continue; mv -- "$f" .; done
	jw swapped
	rm -rf -- "$L/restore-$id"
	echo swapped ;;
rollback)
	[ -e "$j" ] || { echo none; exit 0; }
	read -r jid jpre phase < "$j"
	p="$L/$jpre"; d="$L/discarded-$(date -u +%Y%m%dT%H%M%SZ)-$jid"
	if [ "$phase" != aside ]; then
		mkdir -p "$d"; chmod 700 "$d"
		for f in * .[!.]* ..?*; do { [ -e "$f" ] || [ -L "$f" ]; } || continue; case "$f" in tmp|.levelrail) continue;; esac; mv -- "$f" "$d/"; done
	fi
	if [ -d "$p" ]; then
		for f in "$p"/* "$p"/.[!.]* "$p"/..?*; do { [ -e "$f" ] || [ -L "$f" ]; } || continue
			b="$(basename -- "$f")"
			if [ -e "./$b" ] || [ -L "./$b" ]; then mkdir -p "$d"; mv -- "./$b" "$d/"; fi
			mv -- "$f" .
		done
		rmdir -- "$p"
	fi
	rm -f -- "$j"; sync
	echo rolledback ;;
state)
	if [ -e "$j" ]; then cat "$j"; else echo none; fi ;;
commit)
	rm -f -- "$j"; sync
	n=$(ls -1d "$L"/pre-restore-* 2>/dev/null | wc -l); x=$((n - arg))
	if [ "$x" -gt 0 ]; then ls -1d "$L"/pre-restore-* | sort | head -n "$x" | while read -r o; do rm -rf -- "$o"; done; fi
	echo committed ;;
sweep)
	[ ! -e "$j" ] || { echo busy; exit 0; }
	find "$L" -mindepth 1 -maxdepth 1 \( -name 'restore-*' -o -name 'discarded-*' \) -mmin "+$arg" -exec rm -rf -- {} +
	if [ -d tmp/restore ]; then find tmp/restore -mindepth 1 -maxdepth 1 -mmin "+$arg" -exec rm -rf -- {} +; fi
	echo swept ;;
*) echo "unknown mode $mode" >&2; exit 2 ;;
esac
`

// Root is one volume a restore swaps: Dir is its mount path and Sub the
// matching folder inside the staged restore ("data" or "config").
type Root struct{ Dir, Sub string }

// Volumes names the mail data volume and, when present, the config volume.
type Volumes struct{ Data, Config string }

// StageDir is where a restore id is staged inside root.
func StageDir(root, id string) string { return root + "/" + WorkDir + "/restore-" + id }

// PreRestoreName is a unique pre-restore folder name; it sorts by time.
func PreRestoreName(id string, now time.Time) string {
	return "pre-restore-" + now.UTC().Format("20060102T150405.000000000Z") + "-" + id
}

// HelperName is the restore helper's container name. Underscores keep it
// outside every service's "<name>-" container prefix.
func HelperName(app string) string { return "mail_restore_helper_" + app }

// Helper is a stopped-mail helper container that mounts only the mail volumes
// and has no network.
type Helper struct {
	rt docker.Runtime
	id string
}

// StartHelper removes a helper left over from a crash, then creates and starts a new one.
func StartHelper(ctx context.Context, rt docker.Runtime, image, app string, vols Volumes, life time.Duration) (*Helper, error) {
	if vols.Data == "" {
		return nil, errors.New("moxrun: the mail data volume is unknown")
	}
	name := HelperName(app)
	if st, err := rt.InspectByName(ctx, name); err == nil && st != nil {
		if err := rt.Remove(ctx, st.ID, true); err != nil {
			return nil, fmt.Errorf("remove leftover restore helper: %w", err)
		}
	}
	mounts := []docker.VolumeMount{{Name: vols.Data, ContainerPath: DataDir}}
	if vols.Config != "" {
		mounts = append(mounts, docker.VolumeMount{Name: vols.Config, ContainerPath: ConfigDir})
	}
	hid, err := rt.Create(ctx, docker.ContainerSpec{
		Name: name, Image: image, NetworkMode: "none",
		Entrypoint: []string{"sh", "-c"}, Command: []string{"sleep " + strconv.Itoa(int(life.Seconds()))},
		Volumes: mounts,
	})
	if err != nil {
		return nil, fmt.Errorf("create restore helper: %w", err)
	}
	h := &Helper{rt: rt, id: hid}
	if err := rt.Start(ctx, hid); err != nil {
		h.Close()
		return nil, fmt.Errorf("start restore helper: %w", err)
	}
	return h, nil
}

// Run runs one swapScript mode against root and returns its last output line.
func (h *Helper) Run(ctx context.Context, mode, root string, args ...string) (string, error) {
	rc, err := h.rt.Exec(ctx, h.id, append([]string{"sh", "-c", swapScript, "swap", mode, root}, args...))
	if err != nil {
		return "", fmt.Errorf("restore helper %s %s: %w", mode, root, err)
	}
	out, err := drain(rc)
	_ = rc.Close()
	if err != nil {
		return out, fmt.Errorf("restore helper %s %s: %w", mode, root, err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1]), nil
}

// Sweep removes stale staged and discarded restore dirs under root through
// the running mail container; it does nothing while a swap journal is open.
func Sweep(ctx context.Context, rt Execer, containerID, root, minutes string) error {
	rc, err := rt.Exec(ctx, containerID, []string{"sh", "-c", swapScript, "swap", "sweep", root, "", "", minutes})
	if err != nil {
		return err
	}
	_, err = drain(rc)
	_ = rc.Close()
	return err
}

// Close removes the helper container.
func (h *Helper) Close() { _ = h.rt.Remove(context.Background(), h.id, true) }

// ScriptForTest exposes swapScript to the throwaway-container proof.
func ScriptForTest() string { return swapScript }
