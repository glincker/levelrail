package moxrun

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// labScript builds fixtures, hashes trees and installs fault shims inside the
// helper. A shim counts every external command the swap runs and, at call
// FAIL_AT (counted under a lock, pipelines race), fails it, kills every process but init, or hangs for a docker kill.
const labScript = `set -eu
mk() { mkdir -p "$(dirname -- "$1")"; printf '%s' "$2" > "$1"; }
h() { ( cd "$1" && find . -mindepth 1 \( -path ./.levelrail -o -path ./tmp \) -prune -o -print | sort | while IFS= read -r f; do
	if [ -L "$f" ]; then echo "L $f $(readlink "$f")"; elif [ -f "$f" ]; then echo "F $f $(stat -c %a "$f") $(sha256sum < "$f")"; else echo "D $f $(stat -c %a "$f")"; fi
done ) | sha256sum | cut -c1-16; }
stage() { S="$1/.levelrail/restore-$3/$2"; mk "$S/accounts/new acct/msg" "$4"; mk "$S/.newhidden" "$4"; ln -s accounts "$S/acc link"; mk "$S/tmp/must-not-nest" z; mk "$S/nest/x" "$4"; }
case "$1" in
shims) mkdir -p /shim
	for c in mv mkdir chmod chown sync rm rmdir ls awk date basename cat; do
		printf '%s\n' '#!/bin/sh' 'B=/bin/busybox' \
		'until $B mkdir /tmp/shimlock 2>/dev/null; do :; done' \
		'n=$($B cat /tmp/shimn 2>/dev/null || echo 0); n=$((n+1)); echo "$n" > /tmp/shimn; $B rmdir /tmp/shimlock' \
		'if [ "$n" = "${FAIL_AT:-0}" ]; then case "${FAIL_MODE:-fail}" in' \
		'kill) $B kill -9 -1; exit 99;;' \
		'hang) : > /tmp/hung; exec $B sleep 300;;' \
		'*) echo "injected failure at call $n: ${0##*/} $*" >&2; exit 1;; esac; fi' \
		'exec $B "${0##*/}" "$@"' > "/shim/$c"; chmod +x "/shim/$c"; done ;;
prep) R="$2"; find "$R" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
	mk "$R/accounts/a b/msg 1" "old $R"; mk "$R/.hidden" h; mk "$R/..dots" d; mk "$R/-dash" x; mk "$R/nest/deep/er/f" f; chmod 640 "$R/.hidden"
	ln -s nest/deep "$R/link"; ln -s /etc/hostname "$R/abs link"; mk "$R/tmp/keep" t; mk "$R/.levelrail/earlier/note" e
	stage "$R" "$3" "$4" "$5"; h "$R" ;;
stage) stage "$2" "$3" "$4" "$5" ;;
hash) h "$2" ;;
check) R="$2"; n=flat; { [ -e "$R/tmp/tmp" ] || [ -e "$R/tmp/must-not-nest" ]; } && n=nested; e=lost; [ -e "$R/.levelrail/earlier/note" ] && e=kept
	echo "$(h "$R") $(cat "$R/tmp/keep" 2>/dev/null || echo gone) $n $e" ;;
exists) if [ -e "$2" ] || [ -L "$2" ]; then echo yes; else echo no; fi ;;
calls) cat /tmp/shimn 2>/dev/null || echo 0 ;;
esac
`

type swapLab struct {
	t                    *testing.T
	img, name, data, cfg string
}

func (l *swapLab) docker(stdin string, args ...string) (string, error) {
	cmd := exec.Command("docker", args...) //nolint:gosec // the opt-in proof drives the local docker CLI
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func (l *swapLab) must(stdin string, args ...string) string {
	l.t.Helper()
	out, err := l.docker(stdin, args...)
	if err != nil {
		l.t.Fatalf("docker %v: %v\n%s", args[:2], err, out)
	}
	return out
}

// start replaces the helper, as StartHelper does after a crash.
func (l *swapLab) start() {
	_, _ = l.docker("", "rm", "-f", l.name)
	l.must("", "run", "-d", "--name", l.name, "--network", "none", "-v", l.data+":"+DataDir, "-v", l.cfg+":"+ConfigDir, "--entrypoint", "sleep", l.img, "3600")
	l.must(labScript, "exec", "-i", l.name, "sh", "-c", "cat > /lab.sh && sh /lab.sh shims")
}

func (l *swapLab) lab(args ...string) string {
	l.t.Helper()
	return l.must("", append([]string{"exec", l.name, "sh", "/lab.sh"}, args...)...)
}

// run runs one swapScript mode through the shims, injecting fault at call at.
func (l *swapLab) run(at int, fault, mode, root string, args ...string) (string, error) {
	_, _ = l.docker("", "exec", l.name, "/bin/busybox", "rm", "-rf", "/tmp/shimn", "/tmp/hung", "/tmp/shimlock")
	a := []string{"exec", "-e", "PATH=/shim:/usr/sbin:/usr/bin:/sbin:/bin", "-e", "FAIL_AT=" + strconv.Itoa(at), "-e", "FAIL_MODE=" + fault, l.name, "sh", "-c", swapScript, "swap", mode, root}
	out, err := l.docker("", append(a, args...)...)
	lines := strings.Split(out, "\n")
	return strings.TrimSpace(lines[len(lines)-1]), err
}

func (l *swapLab) ok(mode, root string, args ...string) string {
	l.t.Helper()
	out, err := l.run(0, "", mode, root, args...)
	if err != nil {
		l.t.Fatalf("%s %s: %v %s", mode, root, err, out)
	}
	return out
}

// recoverRoots mirrors mailSwap.recover: a closed data journal commits, anything else rolls back.
func (l *swapLab) recoverRoots(roots ...string) {
	l.t.Helper()
	if l.ok("state", DataDir) == "none" {
		for _, r := range roots {
			l.ok("commit", r, "", "", "2")
		}
		return
	}
	for i := len(roots) - 1; i >= 0; i-- {
		l.ok("rollback", roots[i])
	}
}

func (l *swapLab) wantTree(scenario, root, hash string) {
	l.t.Helper()
	if got, want := l.lab("check", root), hash+" t flat kept"; got != want {
		l.t.Fatalf("%s: %s is %q, want %q", scenario, root, got, want)
	}
	if st := l.ok("state", root); st != "none" {
		l.t.Fatalf("%s: journal still open on %s: %q", scenario, root, st)
	}
}

// TestLiveRestoreSwap proves swapScript under the mox image's busybox on real
// volumes: success, a second restore with the same id, a failure and a crash
// at every external command of the swap and of the rollback, and docker kill.
func TestLiveRestoreSwap(t *testing.T) {
	img := os.Getenv("MOXRUN_LIVE_SWAP_IMAGE")
	if img == "" {
		t.Skip("set MOXRUN_LIVE_SWAP_IMAGE to the pinned mox image (internal/catalog/templates_mail.go)")
	}
	sfx := strconv.FormatInt(time.Now().UnixNano()%1e9, 36)
	l := &swapLab{t: t, img: img, name: "mjtest-swap-" + sfx, data: "mjtest-swap-data-" + sfx, cfg: "mjtest-swap-config-" + sfx}
	t.Cleanup(func() {
		_, _ = l.docker("", "rm", "-f", l.name)
		_, _ = l.docker("", "volume", "rm", "-f", l.data, l.cfg)
	})
	l.must("", "volume", "create", l.data)
	l.must("", "volume", "create", l.cfg)
	l.start()

	roots := []Root{{Dir: DataDir, Sub: "data"}, {Dir: ConfigDir, Sub: "config"}}
	orig, staged := map[string]string{}, map[string]string{}
	for _, r := range roots {
		orig[r.Dir] = l.lab("prep", r.Dir, r.Sub, "ID1", "v1")
		staged[r.Dir] = l.lab("hash", StageDir(r.Dir, "ID1")+"/"+r.Sub)
	}
	p1 := PreRestoreName("ID1", time.Now())
	for _, r := range roots {
		if out := l.ok("swap", r.Dir, "ID1", p1, r.Sub); out != "swapped" {
			t.Fatalf("success: swap %s said %q", r.Dir, out)
		}
	}
	if st := l.ok("state", DataDir); st != "ID1 "+p1+" swapped" {
		t.Fatalf("success: journal before commit is %q", st)
	}
	for _, r := range roots {
		l.ok("commit", r.Dir, "", "", "2")
		l.wantTree("success", r.Dir, staged[r.Dir])
		if got := l.lab("hash", r.Dir+"/"+WorkDir+"/"+p1); got != orig[r.Dir] {
			t.Fatalf("success: %s pre-restore copy %s, want %s", r.Dir, got, orig[r.Dir])
		}
		if l.lab("exists", StageDir(r.Dir, "ID1")) != "no" {
			t.Fatalf("success: staging left on %s", r.Dir)
		}
	}

	prev := staged[DataDir]
	l.lab("stage", DataDir, "data", "ID1", "v2")
	want := l.lab("hash", StageDir(DataDir, "ID1")+"/data")
	time.Sleep(time.Millisecond)
	p2 := PreRestoreName("ID1", time.Now())
	l.ok("swap", DataDir, "ID1", p2, "data")
	l.ok("commit", DataDir, "", "", "2")
	l.wantTree("second restore", DataDir, want)
	if l.lab("hash", DataDir+"/"+WorkDir+"/"+p2) != prev || l.lab("hash", DataDir+"/"+WorkDir+"/"+p1) != orig[DataDir] {
		t.Fatal("second restore with the same id: pre-restore copies collided or changed")
	}

	swapCalls := 0
	for _, fault := range []string{"fail", "kill"} {
		for i := 1; ; i++ {
			o := l.lab("prep", DataDir, "data", "ID1", "v1")
			out, err := l.run(i, fault, "swap", DataDir, "ID1", fmt.Sprintf("pre-restore-%04d-ID1", i), "data")
			if err == nil {
				swapCalls = i - 1
				break
			}
			if fault == "fail" {
				l.ok("rollback", DataDir)
			} else {
				l.recoverRoots(DataDir, ConfigDir)
			}
			l.wantTree(fmt.Sprintf("swap %s at call %d (%s)", fault, i, out), DataDir, o)
		}
		t.Logf("swap %s at each of %d calls: original tree restored", fault, swapCalls)
	}

	for _, fault := range []string{"fail", "kill"} {
		for i := 1; ; i++ {
			o := l.lab("prep", DataDir, "data", "ID1", "v1")
			l.ok("swap", DataDir, "ID1", fmt.Sprintf("pre-restore-%04d-ID1", i), "data")
			out, err := l.run(i, fault, "rollback", DataDir)
			if err == nil {
				l.wantTree("clean rollback", DataDir, o)
				t.Logf("rollback %s at each of %d calls, then recovery: original tree restored", fault, i-1)
				break
			}
			l.recoverRoots(DataDir, ConfigDir)
			l.wantTree(fmt.Sprintf("rollback %s at call %d (%s)", fault, i, out), DataDir, o)
		}
	}

	for _, i := range []int{swapCalls / 3, swapCalls * 2 / 3, swapCalls} {
		o := l.lab("prep", DataDir, "data", "ID1", "v1")
		done := make(chan struct{})
		go func() {
			_, _ = l.run(i, "hang", "swap", DataDir, "ID1", fmt.Sprintf("pre-restore-k%04d-ID1", i), "data")
			close(done)
		}()
		hung := false
		for range 100 {
			if _, err := l.docker("", "exec", l.name, "test", "-e", "/tmp/hung"); err == nil {
				hung = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !hung {
			t.Fatalf("docker kill at call %d: the swap never reached it", i)
		}
		l.must("", "kill", l.name)
		<-done
		l.start()
		l.recoverRoots(DataDir, ConfigDir)
		l.wantTree(fmt.Sprintf("docker kill at call %d", i), DataDir, o)
	}
	t.Logf("docker kill at calls %d, %d, %d, then recovery in a new helper: original tree restored", swapCalls/3, swapCalls*2/3, swapCalls)
}
