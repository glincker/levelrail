package dnszones

import (
	"encoding/json"
	"errors"
	"strings"
)

// GoogleCloudIdentity is the non-secret part of a service account key.
type GoogleCloudIdentity struct {
	ProjectID   string
	ClientEmail string
}

// ParseGoogleCloudKey checks that raw is a service account JSON key and
// returns its project id and client email. Errors never echo the key.
func ParseGoogleCloudKey(raw string) (GoogleCloudIdentity, error) {
	var k struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		PrivateKey  string `json:"private_key"`
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &k); err != nil {
		return GoogleCloudIdentity{}, errors.New("service account key must be a JSON file")
	}
	switch {
	case k.Type != "service_account":
		return GoogleCloudIdentity{}, errors.New(`service account key must have "type": "service_account"`)
	case k.ProjectID == "":
		return GoogleCloudIdentity{}, errors.New("service account key is missing project_id")
	case k.ClientEmail == "":
		return GoogleCloudIdentity{}, errors.New("service account key is missing client_email")
	case !strings.Contains(k.PrivateKey, "PRIVATE KEY"):
		return GoogleCloudIdentity{}, errors.New("service account key is missing private_key")
	}
	return GoogleCloudIdentity{ProjectID: k.ProjectID, ClientEmail: k.ClientEmail}, nil
}
