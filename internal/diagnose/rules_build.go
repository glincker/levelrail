package diagnose

import (
	"fmt"
	"regexp"
)

// GitSourceBuildPathField is the Change field a Dockerfile path fix edits.
const GitSourceBuildPathField = "git_source.build_path"

const maxDockerfileFixes = 3

var dockerfileMissingRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)failed to read dockerfile`),
	regexp.MustCompile(`(?i)cannot locate specified dockerfile`),
	regexp.MustCompile(`(?i)(?:open|read) [^\n]*dockerfile[^\n]*no such file or directory`),
	regexp.MustCompile(`(?i)dockerfile[^\n]*(?:not found|does not exist)`),
}

func ruleWrongDockerfilePath(in Input, lines []line) *Cause {
	f := in.Facts
	if !f.BuildFailed || f.BuildType != "dockerfile" {
		return nil
	}
	l, _, ok := firstMatchAny(lines, dockerfileMissingRes)
	if !ok {
		return nil
	}
	cause := &Cause{
		Code:        CauseWrongDockerfilePath,
		Title:       "The build cannot find the Dockerfile",
		Explanation: fmt.Sprintf("The build looked for %q in the repository and it is not there, so nothing was built.", displayPath(f.BuildPath)),
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
	}
	for _, c := range f.DockerfileCandidates {
		if c == f.BuildPath || len(cause.Fixes) == maxDockerfileFixes {
			continue
		}
		cause.Fixes = append(cause.Fixes, Fix{
			Label:    fmt.Sprintf("Build from %s instead", c),
			Kind:     FixPatch,
			Changes:  []Change{{Field: GitSourceBuildPathField, From: f.BuildPath, To: c}},
			Redeploy: true,
		})
	}
	if len(cause.Fixes) == 0 {
		cause.Fixes = []Fix{{
			Label: "Set the Dockerfile path in the app's build settings",
			Kind:  FixManual,
			Hint:  "No other Dockerfile was found in the repository. Commit one, or switch the build method to Railpack.",
		}}
	}
	return cause
}

func displayPath(p string) string {
	if p == "" {
		return "Dockerfile"
	}
	return p
}
