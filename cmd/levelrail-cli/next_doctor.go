package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	nextSevError = "error"
	nextSevWarn  = "warn"
	nextSevInfo  = "info"

	nextDoctorLargeMBEnv     = "LEVELRAIL_NEXT_DOCTOR_LARGE_MB"
	nextDoctorMinVersionEnv  = "LEVELRAIL_NEXT_DOCTOR_MIN_MAJOR"
	nextDoctorLargeMBDefault = 200
	// Next 14 is where the shared cache handler contract and server action keys are documented.
	nextDoctorMinMajorDefault = 14
)

// nextFinding is one doctor result; Fix never contains values read from the project.
type nextFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

type nextDoctorReport struct {
	Dir      string        `json:"dir"`
	Findings []nextFinding `json:"findings"`
	Errors   int           `json:"errors"`
}

var (
	nextOutputRe    = regexp.MustCompile(`output\s*:\s*['"` + "`" + `]standalone['"` + "`" + `]`)
	nextUnoptRe     = regexp.MustCompile(`unoptimized\s*:\s*true`)
	nextArgIDRe     = regexp.MustCompile(`(?mi)^\s*ARG\s+NEXT_DEPLOYMENT_ID\b`)
	nextPublicRe    = regexp.MustCompile(`NEXT_PUBLIC_[A-Z0-9_]+`)
	nextSecretish   = regexp.MustCompile(`SECRET|PRIVATE|TOKEN|KEY`)
	nextMajorRe     = regexp.MustCompile(`\d+`)
	nextConfigNames = []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"}
)

func nextDoctorUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s next doctor [dir] [--json] [--min-major N] [--large-mb N]

Checks a Next.js project folder before you deploy it. Nothing is sent anywhere
and no server is needed. Each finding has a severity (error, warn, info) and
the exact fix. Exit code 1 when any finding is an error. Values of NEXT_PUBLIC_
variables are never printed, only their names.

Flags:
  --json          print the report as JSON and nothing else
  --min-major N   oldest supported Next.js major (default: %[2]s env var, then %[3]d)
  --large-mb N    upload-folder size that counts as large (default: %[4]s env var, then %[5]d)
  -h, --help      show this help
`, prog, nextDoctorMinVersionEnv, nextDoctorMinMajorDefault, nextDoctorLargeMBEnv, nextDoctorLargeMBDefault)
}

func envIntDefault(lookupEnv func(string) (string, bool), key string, def int) int {
	if v, ok := lookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func runNext(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, nextDoctorUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, nextDoctorUsage(prog))
		return exitOK
	case "doctor":
		return runNextDoctor(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown next subcommand %q\n\n%s", prog, args[0], nextDoctorUsage(prog))
		return exitUsage
	}
}

func runNextDoctor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	flags := flag.NewFlagSet(prog+" next doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var jsonOut bool
	minMajor := flags.Int("min-major", envIntDefault(lookupEnv, nextDoctorMinVersionEnv, nextDoctorMinMajorDefault), "oldest supported Next.js major")
	largeMB := flags.Int("large-mb", envIntDefault(lookupEnv, nextDoctorLargeMBEnv, nextDoctorLargeMBDefault), "upload folder size that counts as large")
	flags.BoolVar(&jsonOut, "json", false, "print the report as JSON")
	flags.Usage = func() { _, _ = fmt.Fprint(stderr, nextDoctorUsage(prog)) }
	// Flags may follow the directory, so split them out first.
	var flagArgs, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json" || a == "-json" || a == "-h" || a == "--help":
			flagArgs = append(flagArgs, a)
		case strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && i+1 < len(args):
			flagArgs = append(flagArgs, a, args[i+1])
			i++
		case strings.HasPrefix(a, "-"):
			flagArgs = append(flagArgs, a)
		default:
			pos = append(pos, a)
		}
	}
	if err := flags.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if len(pos) > 1 {
		_, _ = fmt.Fprint(stderr, nextDoctorUsage(prog))
		return exitUsage
	}
	dir := "."
	if len(pos) == 1 {
		dir = pos[0]
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() { //nolint:gosec // the directory is the one the user asked us to inspect
		_, _ = fmt.Fprintf(stderr, "%s: %q is not a directory\n", prog, dir)
		return exitUsage
	}

	rep := checkNextProject(dir, *minMajor, int64(*largeMB)*1024*1024)
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printNextDoctor(stdout, rep)
	}
	if rep.Errors > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func printNextDoctor(out io.Writer, rep nextDoctorReport) {
	if len(rep.Findings) == 0 {
		_, _ = fmt.Fprintf(out, "%s looks ready to deploy. No findings.\n", rep.Dir)
		return
	}
	for _, sev := range []string{nextSevError, nextSevWarn, nextSevInfo} {
		for _, f := range rep.Findings {
			if f.Severity != sev {
				continue
			}
			_, _ = fmt.Fprintf(out, "[%s] %s\n       fix: %s\n", f.Severity, f.Message, f.Fix)
		}
	}
	_, _ = fmt.Fprintf(out, "\n%d finding(s), %d error(s).\n", len(rep.Findings), rep.Errors)
}

func readTextFile(path string) (string, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // path is built from the directory the user passed
	if err != nil {
		return "", false
	}
	return string(b), true
}

func checkNextProject(dir string, minMajor int, largeBytes int64) nextDoctorReport {
	rep := nextDoctorReport{Dir: dir}
	add := func(id, sev, msg, fix string) {
		rep.Findings = append(rep.Findings, nextFinding{ID: id, Severity: sev, Message: msg, Fix: fix})
		if sev == nextSevError {
			rep.Errors++
		}
	}

	pkgText, ok := readTextFile(filepath.Join(dir, "package.json"))
	if !ok {
		add("no-package-json", nextSevError, "No package.json in this folder.", "Run the command from the project root, or pass the project folder: next doctor <dir>.")
		return rep
	}
	var pkg struct {
		Deps    map[string]string `json:"dependencies"`
		DevDeps map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(pkgText), &pkg); err != nil {
		add("bad-package-json", nextSevError, "package.json is not valid JSON.", "Fix the syntax in package.json, then run the check again.")
		return rep
	}
	nextVer, hasNext := pkg.Deps["next"]
	if !hasNext {
		nextVer, hasNext = pkg.DevDeps["next"]
	}
	if !hasNext {
		add("no-next", nextSevError, "next is not listed in package.json.", "This does not look like a Next.js project. Add next to dependencies if it is one.")
		return rep
	}
	major := 0
	if m := nextMajorRe.FindString(nextVer); m != "" {
		major, _ = strconv.Atoi(m)
	}
	if major > 0 && major < minMajor {
		add("old-next", nextSevError, fmt.Sprintf("Next.js %d is older than the supported floor (%d).", major, minMajor), fmt.Sprintf("Upgrade with: npm install next@%d react react-dom.", minMajor))
	}

	cfg := ""
	for _, n := range nextConfigNames {
		if t, ok := readTextFile(filepath.Join(dir, n)); ok {
			cfg = t
			break
		}
	}
	if !nextOutputRe.MatchString(cfg) {
		add("no-standalone", nextSevWarn, "next.config does not set output: 'standalone'.", "Add output: 'standalone' to next.config for a smaller image and a faster start.")
	}
	if !strings.Contains(cfg, "cacheHandler") {
		add("no-cache-handler", nextSevWarn, "No cacheHandler configured. With more than one replica, ISR pages and data cache would diverge.", "Run: levelrail-cli apps next-cache set <app> redis (see docs/nextjs-shared-cache.md). Ignore this on a single replica.")
	}

	if df, ok := readTextFile(filepath.Join(dir, "Dockerfile")); ok && !nextArgIDRe.MatchString(df) {
		add("no-deployment-id-arg", nextSevWarn, "Dockerfile has no ARG NEXT_DEPLOYMENT_ID, so skew protection will not apply.", "Add ARG NEXT_DEPLOYMENT_ID and ENV NEXT_DEPLOYMENT_ID=$NEXT_DEPLOYMENT_ID before the next build step (see docs/nextjs-skew-protection.md).")
	}

	_, sharpDep := pkg.Deps["sharp"]
	if !sharpDep && !nextUnoptRe.MatchString(cfg) {
		sev := nextSevWarn
		if major >= 15 {
			sev = nextSevInfo
		}
		add("no-sharp", sev, "sharp is not a dependency, so image optimisation may fall back to a slower path.", "Run: npm install sharp.")
	}

	checkNextUploadDir(dir, largeBytes, add)
	checkNextPublicSecrets(dir, cfg, add)
	return rep
}

func ignoreListed(text, name string) bool {
	for _, line := range strings.Split(text, "\n") {
		l := strings.Trim(strings.TrimSpace(line), "/")
		l = strings.TrimSuffix(l, "/**")
		if l == name || l == "**/"+name {
			return true
		}
	}
	return false
}

func checkNextUploadDir(dir string, largeBytes int64, add func(id, sev, msg, fix string)) {
	dockerignore, hasDI := readTextFile(filepath.Join(dir, ".dockerignore"))
	gitignore, _ := readTextFile(filepath.Join(dir, ".gitignore"))
	if !hasDI {
		add("no-dockerignore", nextSevWarn, "No .dockerignore, so the upload and build context include everything.", "Create .dockerignore listing node_modules, .next and .git.")
	}
	for _, name := range []string{"node_modules", ".next"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil { //nolint:gosec // the directory is the one the user asked us to inspect
			continue
		}
		if ignoreListed(dockerignore, name) || ignoreListed(gitignore, name) {
			continue
		}
		if dirExceeds(filepath.Join(dir, name), largeBytes) {
			add("large-"+strings.TrimPrefix(name, "."), nextSevWarn, fmt.Sprintf("%s is large and not in .dockerignore or .gitignore, so it would be uploaded.", name), fmt.Sprintf("Add %s to .dockerignore.", name))
		}
	}
}

// dirExceeds stops walking as soon as the limit is passed so node_modules is never fully scanned.
func dirExceeds(root string, limit int64) bool {
	var total int64
	errStop := fmt.Errorf("limit reached")
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error { //nolint:gosec // the directory is the one the user asked us to inspect
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		if info, ierr := d.Info(); ierr == nil {
			total += info.Size()
		}
		if total > limit {
			return errStop
		}
		return nil
	})
	return errors.Is(err, errStop)
}

func checkNextPublicSecrets(dir, cfg string, add func(id, sev, msg, fix string)) {
	sources := []string{cfg}
	if df, ok := readTextFile(filepath.Join(dir, "Dockerfile")); ok {
		sources = append(sources, df)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), ".env") && !strings.HasSuffix(e.Name(), ".example") && !strings.HasSuffix(e.Name(), ".sample") {
			if t, ok := readTextFile(filepath.Join(dir, e.Name())); ok {
				sources = append(sources, t)
			}
		}
	}
	seen := map[string]bool{}
	for _, src := range sources {
		for _, name := range nextPublicRe.FindAllString(src, -1) {
			if seen[name] || !nextSecretish.MatchString(name) || strings.Contains(name, "PUBLISHABLE") {
				continue
			}
			seen[name] = true
			sev := nextSevWarn
			if strings.Contains(name, "SECRET") || strings.Contains(name, "PRIVATE") || strings.Contains(name, "TOKEN") {
				sev = nextSevError
			}
			add("public-secret-"+strings.ToLower(name), sev,
				fmt.Sprintf("%s is exposed to every visitor: NEXT_PUBLIC_ values are baked into the browser bundle and its name suggests a secret.", name),
				fmt.Sprintf("Rename it without the NEXT_PUBLIC_ prefix and read it only in server code, or confirm %s is safe to publish.", name))
		}
	}
}
