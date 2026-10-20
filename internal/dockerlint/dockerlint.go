// Package dockerlint checks a Dockerfile for the handful of hints that most
// often cause production trouble and offers a safe patch where one exists.
package dockerlint

import (
	"regexp"
	"strings"
)

// Rule IDs and severities, shared by the API, CLI and UI.
const (
	RuleRootUser    = "root-user"
	RuleHealthcheck = "no-healthcheck"
	RuleMultiStage  = "single-stage-build"
	RuleDockerignor = "no-dockerignore"
	RuleFloatingTag = "floating-base-tag"

	SeverityWarn = "warn"
	SeverityInfo = "info"

	// NonRootUser is a numeric UID:GID so the patch works on images that ship no named user.
	NonRootUser = "65532:65532"
)

// Finding is one hint. Line is 1-based, 0 when it applies to the whole file.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Fixable  bool   `json:"fixable"`
}

// Result is the lint output plus the patched file and suggested .dockerignore.
type Result struct {
	Findings         []Finding `json:"findings"`
	FixedDockerfile  string    `json:"fixed_dockerfile"`
	DockerignoreHint string    `json:"dockerignore_suggestion,omitempty"`
}

type instruction struct {
	op   string
	args string
	line int
}

var buildCmd = regexp.MustCompile(`(?i)\b(npm (run )?build|yarn build|pnpm( run)? build|go build|cargo build|mvn |gradle |dotnet publish)\b`)

const dockerignoreSuggestion = ".git\nnode_modules\n.env\n.env.*\n*.log\nDockerfile\n.dockerignore\n"

func parse(src string) []instruction {
	var out []instruction
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		start := i + 1
		raw := strings.TrimSpace(lines[i])
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		for strings.HasSuffix(raw, "\\") && i+1 < len(lines) {
			i++
			next := strings.TrimSpace(lines[i])
			if strings.HasPrefix(next, "#") {
				continue
			}
			raw = strings.TrimSpace(strings.TrimSuffix(raw, "\\")) + " " + next
		}
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "\\"))
		op, args, _ := strings.Cut(raw, " ")
		out = append(out, instruction{op: strings.ToUpper(op), args: strings.TrimSpace(args), line: start})
	}
	return out
}

func baseRef(args string) (ref, stage string) {
	fields := strings.Fields(args)
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return "", ""
	}
	ref = fields[0]
	if len(fields) >= 3 && strings.EqualFold(fields[1], "as") {
		stage = strings.ToLower(fields[2])
	}
	return ref, stage
}

func floatingTag(ref string, stages map[string]bool) bool {
	lower := strings.ToLower(ref)
	if ref == "" || lower == "scratch" || stages[lower] || strings.Contains(ref, "@") || strings.Contains(ref, "$") {
		return false
	}
	last := ref[strings.LastIndex(ref, "/")+1:]
	_, tag, hasTag := strings.Cut(last, ":")
	return !hasTag || tag == "latest"
}

func isRoot(user string) bool {
	name, _, _ := strings.Cut(user, ":")
	return name == "" || name == "root" || name == "0"
}

// Lint inspects dockerfile. hasDockerignore says whether the build context ships a .dockerignore.
func Lint(dockerfile string, hasDockerignore bool) Result {
	ins := parse(dockerfile)
	res := Result{Findings: []Finding{}, FixedDockerfile: dockerfile}

	var froms []int
	stages := map[string]bool{}
	for i, in := range ins {
		if in.op != "FROM" {
			continue
		}
		froms = append(froms, i)
		ref, stage := baseRef(in.args)
		if floatingTag(ref, stages) {
			res.Findings = append(res.Findings, Finding{Rule: RuleFloatingTag, Severity: SeverityWarn, Line: in.line,
				Message: "Base image " + ref + " has no pinned tag, so builds change without notice. Pin a version tag or digest."})
		}
		if stage != "" {
			stages[stage] = true
		}
	}
	if len(froms) == 0 {
		return res
	}

	final := ins[froms[len(froms)-1]:]
	user, hasHealth, lastEntry := "", false, -1
	for _, in := range final {
		switch in.op {
		case "USER":
			if f := strings.Fields(in.args); len(f) > 0 {
				user = strings.ToLower(f[0])
			}
		case "HEALTHCHECK":
			hasHealth = !strings.HasPrefix(strings.ToUpper(in.args), "NONE")
		case "CMD", "ENTRYPOINT":
			lastEntry = in.line
		}
	}

	if isRoot(user) {
		res.Findings = append(res.Findings, Finding{Rule: RuleRootUser, Severity: SeverityWarn, Line: final[0].line,
			Message: "The final image runs as root. A compromise of the app then owns the container. Add a non-root USER.", Fixable: true})
		res.FixedDockerfile = insertUser(dockerfile, lastEntry)
	}
	if !hasHealth {
		res.Findings = append(res.Findings, Finding{Rule: RuleHealthcheck, Severity: SeverityInfo,
			Message: "No HEALTHCHECK. Set an app health path in the app settings, or add a HEALTHCHECK so the platform can tell a hung process from a healthy one."})
	}
	if len(froms) == 1 {
		for _, in := range ins {
			if in.op == "RUN" && buildCmd.MatchString(in.args) {
				res.Findings = append(res.Findings, Finding{Rule: RuleMultiStage, Severity: SeverityInfo, Line: in.line,
					Message: "Build tools run in the same stage that ships. A multi-stage build keeps compilers and caches out of the final image."})
				break
			}
		}
	}
	if !hasDockerignore {
		res.Findings = append(res.Findings, Finding{Rule: RuleDockerignor, Severity: SeverityWarn,
			Message: "No .dockerignore. The whole context, including .git and .env files, is sent to the builder and can end up in layers.", Fixable: true})
		res.DockerignoreHint = dockerignoreSuggestion
	}
	return res
}

func insertUser(src string, beforeLine int) string {
	lines := strings.Split(src, "\n")
	stmt := "USER " + NonRootUser
	if beforeLine < 1 || beforeLine > len(lines) {
		return strings.TrimRight(src, "\n") + "\n" + stmt + "\n"
	}
	idx := beforeLine - 1
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, stmt)
	out = append(out, lines[idx:]...)
	return strings.Join(out, "\n")
}
