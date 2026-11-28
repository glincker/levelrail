// Command announce builds, signs and verifies the announcement feed.
// The same rules back CI and the instance side verifier.
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const usage = `usage: announce <command> [flags]

  keygen   -id ID -out FILE                     create an ed25519 key (private key stays offline)
  validate [-prev FEED] [-now T] PATH           check an announcements folder, item file or built feed
  build    -dir DIR [-prev FEED] -out FILE      assemble feed.v1.json with the next sequence
  sign     -feed F -roles R -key K [-key K2]    write the detached signature list
  verify   -feed F -sig S -roles R              apply the instance rules to a signed feed
`

const (
	exitFail    = 1
	exitUsage   = 2
	exitOffline = 3
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errw, usage)
		return exitUsage
	}
	cmds := map[string]func([]string, io.Writer, io.Writer) int{
		"keygen": cmdKeygen, "validate": cmdValidate, "build": cmdBuild, "sign": cmdSign, "verify": cmdVerify,
	}
	fn, ok := cmds[args[0]]
	if !ok {
		fmt.Fprint(errw, usage)
		return exitUsage
	}
	return fn(args[1:], out, errw)
}

func newFlags(name string, errw io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errw)
	return fs
}

func parseNow(s string) (time.Time, error) {
	if s == "" {
		return time.Now().UTC(), nil
	}
	return parseTime(s)
}

func cmdKeygen(args []string, out, errw io.Writer) int {
	fs := newFlags("keygen", errw)
	id := fs.String("id", "", "key id such as S1 or N")
	path := fs.String("out", "", "path for the new key file")
	if fs.Parse(args) != nil || *id == "" || *path == "" {
		fmt.Fprintln(errw, "keygen needs -id and -out")
		return exitUsage
	}
	k, err := newKeyFile(*id, randReader())
	if err == nil {
		err = writeKeyFile(*path, k)
	}
	if err != nil {
		fmt.Fprintln(errw, "keygen:", err)
		return exitFail
	}
	pub, _ := decodeB64(k.PublicKey)
	fmt.Fprintf(out, "key_id:      %s\npublic_key:  %s\nfingerprint: %s\n", k.KeyID, k.PublicKey, fingerprint(pub))
	fmt.Fprintf(errw, "\nWRITTEN %s (mode 0600). This file is the PRIVATE key.\n"+
		"It must live offline: move it to encrypted offline storage now, keep a backup, and never commit, upload or paste it.\n", *path)
	return 0
}

func cmdValidate(args []string, out, errw io.Writer) int {
	fs := newFlags("validate", errw)
	prev := fs.String("prev", "", "previous feed.v1.json, sequence must increase")
	nowS := fs.String("now", "", "override the clock (RFC 3339)")
	if fs.Parse(args) != nil || fs.NArg() != 1 {
		fmt.Fprintln(errw, "validate needs one path")
		return exitUsage
	}
	now, err := parseNow(*nowS)
	if err != nil {
		fmt.Fprintln(errw, "validate: bad -now")
		return exitUsage
	}
	items, probs := validatePath(fs.Arg(0), validateOpts{Now: now, Lim: defaultLimits(), PrevFeed: *prev})
	failed := 0
	for _, p := range probs {
		fmt.Fprintln(errw, p)
		if !p.Warn {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(errw, "validate: %d problem(s)\n", failed)
		return exitFail
	}
	fmt.Fprintf(out, "ok: %d item(s)\n", len(items))
	return 0
}

func cmdBuild(args []string, out, errw io.Writer) int {
	fs := newFlags("build", errw)
	dir := fs.String("dir", "", "announcements folder")
	prev := fs.String("prev", "", "previous feed.v1.json")
	seq := fs.Int64("sequence", 0, "explicit sequence (default previous + 1, or 1)")
	nowS := fs.String("now", "", "override the clock (RFC 3339)")
	dest := fs.String("out", "", "output feed file")
	if fs.Parse(args) != nil || *dir == "" || *dest == "" {
		fmt.Fprintln(errw, "build needs -dir and -out")
		return exitUsage
	}
	now, err := parseNow(*nowS)
	if err != nil {
		fmt.Fprintln(errw, "build: bad -now")
		return exitUsage
	}
	raw, f, err := buildFeed(buildOpts{Dir: *dir, PrevFeed: *prev, Sequence: *seq, Now: now, Lim: defaultLimits()})
	if err == nil {
		err = os.WriteFile(*dest, raw, 0o644)
	}
	if err != nil {
		fmt.Fprintln(errw, "build:", err)
		return exitFail
	}
	tiers := map[string]int{}
	for _, it := range f.Items {
		tiers[it.Tier]++
	}
	fmt.Fprintf(out, "sequence=%d items=%d tiers=%v\n", f.Sequence, len(f.Items), tiers)
	return 0
}

func cmdSign(args []string, out, errw io.Writer) int {
	fs := newFlags("sign", errw)
	var keys multiFlag
	feed := fs.String("feed", "", "built feed file")
	rolesP := fs.String("roles", "", "roles.json with public keys and tier limits")
	dest := fs.String("out", "", "signature file (default FEED.sig)")
	only := fs.String("only-tiers", "", "refuse feeds holding any other tier (exit 3)")
	appendSig := fs.Bool("append", false, "keep existing signatures in -out")
	partial := fs.Bool("partial", false, "skip the tier sufficiency check, for the first of two offline signers")
	fs.Var(&keys, "key", "private key file, repeat for a second signer")
	if fs.Parse(args) != nil || *feed == "" || *rolesP == "" || len(keys) == 0 {
		fmt.Fprintln(errw, "sign needs -feed, -roles and at least one -key")
		return exitUsage
	}
	if *dest == "" {
		*dest = *feed + ".sig"
	}
	raw, err := os.ReadFile(*feed)
	if err != nil {
		fmt.Fprintln(errw, "sign:", err)
		return exitFail
	}
	roles, err := loadRoles(*rolesP)
	if err != nil {
		fmt.Fprintln(errw, "sign:", err)
		return exitFail
	}
	o := signOpts{KeyPaths: keys, Partial: *partial}
	if *only != "" {
		o.OnlyTiers = strings.Split(*only, ",")
	}
	if *appendSig {
		if prev, rerr := os.ReadFile(*dest); rerr == nil {
			if json.Unmarshal(prev, &o.Existing) != nil {
				fmt.Fprintln(errw, "sign: existing signature file is not valid")
				return exitFail
			}
		}
	}
	sigs, err := signFeed(raw, roles, o)
	if err != nil {
		fmt.Fprintln(errw, "sign:", err)
		if errors.Is(err, errNeedsOffline) {
			return exitOffline
		}
		return exitFail
	}
	enc, _ := canonical(sigs)
	if err := os.WriteFile(*dest, enc, 0o644); err != nil {
		fmt.Fprintln(errw, "sign:", err)
		return exitFail
	}
	fmt.Fprintf(out, "signed %s with %d key(s)\n", *feed, len(sigs))
	return 0
}

func cmdVerify(args []string, out, errw io.Writer) int {
	fs := newFlags("verify", errw)
	feed := fs.String("feed", "", "feed.v1.json")
	sig := fs.String("sig", "", "signature file (default FEED.sig)")
	rolesP := fs.String("roles", "", "roles.json with public keys")
	last := fs.Int64("last-sequence", 0, "highest sequence already seen")
	nowS := fs.String("now", "", "override the clock (RFC 3339)")
	revoked := fs.String("revoked", "", "comma separated revoked key ids")
	allowDrops := fs.Bool("allow-drops", false, "exit 0 even when items were dropped")
	asJSON := fs.Bool("json", false, "print the result as json")
	if fs.Parse(args) != nil || *feed == "" || *rolesP == "" {
		fmt.Fprintln(errw, "verify needs -feed and -roles")
		return exitUsage
	}
	if *sig == "" {
		*sig = *feed + ".sig"
	}
	now, nerr := parseNow(*nowS)
	roles, rerr := loadRoles(*rolesP)
	raw, ferr := os.ReadFile(*feed)
	sigRaw, serr := os.ReadFile(*sig)
	if err := errors.Join(nerr, rerr, ferr, serr); err != nil {
		fmt.Fprintln(errw, "verify:", err)
		return exitFail
	}
	rev := map[string]bool{}
	for _, id := range strings.Split(*revoked, ",") {
		if id != "" {
			rev[id] = true
		}
	}
	res := verifyFeed(raw, sigRaw, roles, VerifyOpts{Now: now, LastSequence: *last, Lim: defaultLimits(), Revoked: rev})
	if *asJSON {
		enc, _ := canonical(res)
		_, _ = out.Write(enc)
	} else if res.OK {
		fmt.Fprintf(out, "ok: sequence %d, signers %v, accepted %d, dropped %d\n", res.Sequence, res.Signers, len(res.Accepted), len(res.Dropped))
		for _, d := range res.Dropped {
			fmt.Fprintf(out, "dropped %s: %s %s\n", d.ID, d.Code, d.Detail)
		}
	} else {
		fmt.Fprintf(errw, "rejected: %s: %s\n", res.ErrCode, res.Err)
	}
	if !res.OK || (len(res.Dropped) > 0 && !*allowDrops) {
		return exitFail
	}
	return 0
}

func decodeB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
