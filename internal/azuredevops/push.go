package azuredevops

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/webhook"
)

// nullObjectID is the oldObjectId or newObjectId Azure DevOps reports for a created or deleted ref.
const nullObjectID = "0000000000000000000000000000000000000000"

type pushPayload struct {
	Resource struct {
		Commits []struct {
			Comment string `json:"comment"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commits"`
		RefUpdates []struct {
			Name        string `json:"name"`
			OldObjectID string `json:"oldObjectId"`
			NewObjectID string `json:"newObjectId"`
		} `json:"refUpdates"`
		Repository struct {
			Name      string `json:"name"`
			RemoteURL string `json:"remoteUrl"`
			WebURL    string `json:"webUrl"`
			Project   struct {
				Name string `json:"name"`
			} `json:"project"`
		} `json:"repository"`
	} `json:"resource"`
}

// IsPushPayload reports whether body is an Azure DevOps git.push service hook delivery.
func IsPushPayload(body []byte) bool {
	var p struct {
		EventType string `json:"eventType"`
	}
	return json.Unmarshal(body, &p) == nil && p.EventType == pushEventType
}

// ParsePushEvent decodes a git.push delivery into a webhook.PushEvent. The
// first branch update wins; tag pushes yield webhook.ErrPushEventFieldsMissing.
func ParsePushEvent(body []byte) (webhook.PushEvent, error) {
	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return webhook.PushEvent{}, fmt.Errorf("%s: malformed push payload: %w", errPrefix, err)
	}
	for _, u := range p.Resource.RefUpdates {
		if !strings.HasPrefix(u.Name, "refs/heads/") || u.NewObjectID == "" {
			continue
		}
		ev := webhook.PushEvent{Ref: u.Name, After: u.NewObjectID, Deleted: u.NewObjectID == nullObjectID}
		if u.OldObjectID != nullObjectID {
			ev.Before = u.OldObjectID
		}
		if len(p.Resource.Commits) > 0 {
			head := p.Resource.Commits[0]
			ev.HeadMessage, ev.HeadAuthor, ev.HeadCommitAt = head.Comment, head.Author.Name, head.Author.Date
		}
		repo := p.Resource.Repository
		if repo.Project.Name != "" && repo.Name != "" {
			ev.RepoNames = []string{repo.Project.Name + "/" + repo.Name}
		}
		for _, raw := range []string{repo.RemoteURL, repo.WebURL} {
			if raw != "" {
				ev.RepoURLs = append(ev.RepoURLs, stripUserinfo(raw))
			}
		}
		return ev, nil
	}
	return webhook.PushEvent{}, webhook.ErrPushEventFieldsMissing
}

// VerifyBasicAuth checks the Authorization header of a service hook delivery
// against the shared secret in constant time. A missing or malformed header fails closed.
func VerifyBasicAuth(secret string, header http.Header) bool {
	const prefix = "Basic "
	h := header.Get("Authorization")
	if secret == "" || !strings.HasPrefix(h, prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(h, prefix))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	return ok && user == WebhookUser && subtle.ConstantTimeCompare([]byte(pass), []byte(secret)) == 1
}
