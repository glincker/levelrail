// Command glinr-bot evaluates one pull request against .github/glinr-bot.yml.
// Run on pull_request_target with the base branch checked out; see docs/glinr-bot.md.
// Env: REPO, PR, GH_TOKEN, POLICY (default .github/glinr-bot.yml), MODE, APP_LOGIN.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const commentMarker = "<!-- glinr-bot -->"

type ghPull struct {
	Title string `json:"title"`
	Draft bool   `json:"draft"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Head struct {
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

func gh(args ...string) ([]byte, error) {
	out, err := exec.Command("gh", args...).Output() //nolint:gosec // args are built from the workflow's own env, gh is the only binary run
	if err != nil {
		var detail string
		if ee, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		return out, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(len(args), 3)], " "), err, detail)
	}
	return out, nil
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "glinr-bot: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	repo, pr := os.Getenv("REPO"), os.Getenv("PR")
	if repo == "" || pr == "" {
		fail("REPO and PR are required")
	}
	path := os.Getenv("POLICY")
	if path == "" {
		path = ".github/glinr-bot.yml"
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from the workflow, not the PR
	if err != nil {
		fail("read policy: %v", err)
	}
	policy, err := ParsePolicy(raw)
	if err != nil {
		fail("%v", err)
	}
	if m := os.Getenv("MODE"); m != "" {
		policy.Mode = m
	}

	body, err := gh("api", "repos/"+repo+"/pulls/"+pr)
	if err != nil {
		fail("%v", err)
	}
	var p ghPull
	if err := json.Unmarshal(body, &p); err != nil {
		fail("decode pull request: %v", err)
	}
	filesOut, err := gh("api", "--paginate", "repos/"+repo+"/pulls/"+pr+"/files", "--jq", ".[].filename")
	if err != nil {
		fail("%v", err)
	}
	info := PR{
		Author: p.User.Login, Title: p.Title, Draft: p.Draft,
		Fork:         p.Head.Repo.FullName != p.Base.Repo.FullName,
		Files:        strings.Fields(string(filesOut)),
		ChangedLines: p.Additions + p.Deletions,
		SelfAuthored: p.User.Login == os.Getenv("APP_LOGIN") && p.User.Login != "",
	}
	for _, l := range p.Labels {
		info.Labels = append(info.Labels, l.Name)
	}

	v := Evaluate(policy, info)
	var done []string
	if policy.Mode == ModeEnforce {
		done = act(repo, pr, p.Head.SHA, v, info)
	}
	if err := upsertComment(repo, pr, renderComment(v, policy.Mode, done)); err != nil {
		fail("%v", err)
	}
	fmt.Printf("glinr-bot: %s#%s mode=%s rule=%q actions=%v done=%v\n", repo, pr, policy.Mode, v.Rule, v.Actions, done)
}

// act performs the earned actions that change anything besides the comment.
func act(repo, pr, sha string, v Verdict, info PR) []string {
	var done []string
	for _, a := range v.Actions {
		switch a {
		case ActionApprove:
			if info.SelfAuthored {
				done = append(done, "approve skipped: the bot authored this PR")
				continue
			}
			if alreadyApproved(repo, pr, sha) {
				done = append(done, "approve: already approved this commit")
				continue
			}
			if _, err := gh("pr", "review", pr, "--repo", repo, "--approve", "--body", "Approved by glinr-bot: rule "+v.Rule+" passed."); err != nil {
				done = append(done, "approve failed: "+err.Error())
				continue
			}
			done = append(done, "approved")
		case ActionAutomerge:
			if _, err := gh("pr", "merge", pr, "--repo", repo, "--auto", "--squash"); err != nil {
				done = append(done, "auto-merge failed: "+err.Error())
				continue
			}
			done = append(done, "auto-merge armed, GitHub merges when required checks pass")
		}
	}
	return done
}

func alreadyApproved(repo, pr, sha string) bool {
	out, err := gh("api", "repos/"+repo+"/pulls/"+pr+"/reviews", "--paginate",
		"--jq", `.[] | select(.state=="APPROVED") | select(.user.login | endswith("[bot]")) | .commit_id`)
	if err != nil {
		return false
	}
	for _, c := range strings.Fields(string(out)) {
		if c == sha {
			return true
		}
	}
	return false
}

func upsertComment(repo, pr, body string) error {
	idOut, err := gh("api", "--paginate", "repos/"+repo+"/issues/"+pr+"/comments",
		"--jq", fmt.Sprintf(`.[] | select(.body | contains(%q)) | .id`, commentMarker))
	if err != nil {
		return err
	}
	if id := strings.TrimSpace(strings.SplitN(string(idOut), "\n", 2)[0]); id != "" {
		_, err = gh("api", "-X", "PATCH", "repos/"+repo+"/issues/comments/"+id, "-f", "body="+body)
		return err
	}
	_, err = gh("api", "-X", "POST", "repos/"+repo+"/issues/"+pr+"/comments", "-f", "body="+body)
	return err
}

func renderComment(v Verdict, mode string, done []string) string {
	var b strings.Builder
	b.WriteString(commentMarker + "\n")
	b.WriteString("### glinr-bot\n\n")
	if v.Rule != "" {
		fmt.Fprintf(&b, "Rule **%s** matched. ", v.Rule)
	}
	b.WriteString(v.Reason + "\n\n")
	b.WriteString("| Gate | Result | Detail |\n| --- | --- | --- |\n")
	for _, g := range v.Gates {
		res := "pass"
		if !g.Pass {
			res = "fail"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", g.Name, res, g.Detail)
	}
	b.WriteString("\n")
	switch {
	case mode == ModeShadow && len(v.Actions) > 0 && v.Rule != "":
		fmt.Fprintf(&b, "Shadow mode: would %s. Nothing was changed.\n", strings.Join(withoutComment(v.Actions), " and "))
	case len(done) > 0:
		b.WriteString("Did: " + strings.Join(done, "; ") + ".\n")
	default:
		b.WriteString("No action taken. A person reviews this one.\n")
	}
	return b.String()
}

func withoutComment(actions []string) []string {
	var out []string
	for _, a := range actions {
		if a != ActionComment {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return []string{"only comment"}
	}
	return out
}
