package scim

import (
	"encoding/json"
	"errors"
	"strings"
)

// Schema URNs from RFC 7643 and RFC 7644.
const (
	SchemaUser         = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaGroup        = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SchemaListResponse = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaError        = "urn:ietf:params:scim:api:messages:2.0:Error"
	SchemaPatchOp      = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaSPConfig     = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SchemaResourceType = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	ContentType        = "application/scim+json"
)

// Meta is the common resource metadata.
type Meta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Location     string `json:"location,omitempty"`
}

// Name is the user's structured name.
type Name struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

// Email is one address of a user.
type Email struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// Ref points at another resource, a group member or a user's group.
type Ref struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

// User is the core User resource.
type User struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id,omitempty"`
	ExternalID  string   `json:"externalId,omitempty"`
	UserName    string   `json:"userName"`
	Name        *Name    `json:"name,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Emails      []Email  `json:"emails,omitempty"`
	Active      *bool    `json:"active,omitempty"`
	Groups      []Ref    `json:"groups,omitempty"`
	Meta        *Meta    `json:"meta,omitempty"`
}

// PrimaryEmail is the primary email, else the first, else a userName that is an address.
func (u User) PrimaryEmail() string {
	for _, e := range u.Emails {
		if e.Primary && e.Value != "" {
			return strings.TrimSpace(e.Value)
		}
	}
	for _, e := range u.Emails {
		if e.Value != "" {
			return strings.TrimSpace(e.Value)
		}
	}
	if strings.Contains(u.UserName, "@") {
		return strings.TrimSpace(u.UserName)
	}
	return ""
}

// Group is the core Group resource.
type Group struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id,omitempty"`
	ExternalID  string   `json:"externalId,omitempty"`
	DisplayName string   `json:"displayName"`
	Members     []Ref    `json:"members,omitempty"`
	Meta        *Meta    `json:"meta,omitempty"`
}

// ListHeader is the paging part of a ListResponse.
type ListHeader struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
}

// UserList is a ListResponse of users.
type UserList struct {
	ListHeader
	Resources []User `json:"Resources"`
}

// GroupList is a ListResponse of groups.
type GroupList struct {
	ListHeader
	Resources []Group `json:"Resources"`
}

// NewListHeader describes one page of n items out of total, starting at start.
func NewListHeader(n, total, start int) ListHeader {
	return ListHeader{Schemas: []string{SchemaListResponse}, TotalResults: total, StartIndex: start, ItemsPerPage: n}
}

// Error is the SCIM error body; Status is a string per RFC 7644.
type Error struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// ParseBool accepts a JSON bool or the "True"/"False" strings Entra ID sends.
func ParseBool(raw json.RawMessage) (bool, error) {
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, errors.New("scim: expected a boolean")
}

// Page clamps startIndex and count to RFC 7644 rules and the server maximum.
func Page(startIndex, count, maxCount int) (start, size int) {
	start, size = startIndex, count
	if start < 1 {
		start = 1
	}
	if size <= 0 || size > maxCount {
		size = maxCount
	}
	return start, size
}
