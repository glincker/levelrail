package scim

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidPatch is answered as 400 with scimType invalidValue or invalidPath.
var ErrInvalidPatch = errors.New("scim: invalid patch")

// PatchRequest is a PatchOp message (RFC 7644 section 3.5.2).
type PatchRequest struct {
	Schemas    []string         `json:"schemas"`
	Operations []PatchOperation `json:"Operations"`
}

// PatchOperation is one add, replace or remove.
type PatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

func invalidPatch(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidPatch, fmt.Sprintf(format, args...))
}

func decodePatch(body []byte) ([]PatchOperation, error) {
	var req PatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, invalidPatch("body is not a PatchOp message")
	}
	if len(req.Operations) == 0 {
		return nil, invalidPatch("no Operations")
	}
	for i := range req.Operations {
		op := strings.ToLower(strings.TrimSpace(req.Operations[i].Op))
		if op != "add" && op != "replace" && op != "remove" {
			return nil, invalidPatch("unsupported op %q", req.Operations[i].Op)
		}
		req.Operations[i].Op = op
	}
	return req.Operations, nil
}

// UserChange lists the stored user attributes a PATCH touches; nil means untouched.
type UserChange struct {
	UserName    *string
	DisplayName *string
	GivenName   *string
	FamilyName  *string
	Email       *string
	ExternalID  *string
	Active      *bool
}

// ParseUserPatch folds a user PatchOp into a UserChange. Attributes the
// platform does not store are ignored, so IdPs that send extra profile data still work.
func ParseUserPatch(body []byte) (UserChange, error) {
	ops, err := decodePatch(body)
	if err != nil {
		return UserChange{}, err
	}
	var c UserChange
	for _, op := range ops {
		if op.Path == "" {
			if op.Op == "remove" {
				return UserChange{}, invalidPatch("remove needs a path")
			}
			var attrs map[string]json.RawMessage
			if err := json.Unmarshal(op.Value, &attrs); err != nil {
				return UserChange{}, invalidPatch("value must be an object when path is empty")
			}
			for k, v := range attrs {
				if err := c.set(k, v, false); err != nil {
					return UserChange{}, err
				}
			}
			continue
		}
		if err := c.set(op.Path, op.Value, op.Op == "remove"); err != nil {
			return UserChange{}, err
		}
	}
	return c, nil
}

func (c *UserChange) set(path string, raw json.RawMessage, remove bool) error {
	key := strings.ToLower(strings.TrimSpace(path))
	if remove {
		raw = json.RawMessage(`""`)
	}
	str := func() (*string, error) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, invalidPatch("%s must be a string", path)
		}
		s = strings.TrimSpace(s)
		return &s, nil
	}
	var err error
	switch {
	case key == "active":
		if remove {
			return invalidPatch("active cannot be removed")
		}
		b, perr := ParseBool(raw)
		if perr != nil {
			return invalidPatch("active must be a boolean")
		}
		c.Active = &b
	case key == "username":
		c.UserName, err = str()
	case key == "displayname":
		c.DisplayName, err = str()
	case key == "externalid":
		c.ExternalID, err = str()
	case key == "name.givenname":
		c.GivenName, err = str()
	case key == "name.familyname":
		c.FamilyName, err = str()
	case key == "name":
		if remove {
			empty := ""
			c.GivenName, c.FamilyName = &empty, &empty
			return nil
		}
		var n Name
		if json.Unmarshal(raw, &n) != nil {
			return invalidPatch("name must be an object")
		}
		c.GivenName, c.FamilyName = &n.GivenName, &n.FamilyName
	case strings.HasPrefix(key, "emails"):
		return c.setEmail(raw, remove)
	}
	return err
}

// setEmail accepts an emails array, or the scalar Entra ID sends for emails[type eq "work"].value.
func (c *UserChange) setEmail(raw json.RawMessage, remove bool) error {
	if remove {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		s = strings.TrimSpace(s)
		c.Email = &s
		return nil
	}
	var list []Email
	if json.Unmarshal(raw, &list) != nil {
		return invalidPatch("emails must be a list or a string")
	}
	if e := (User{Emails: list}).PrimaryEmail(); e != "" {
		c.Email = &e
	}
	return nil
}

// MemberOpKind says how a MemberOp changes a group's members.
type MemberOpKind int

// Member operation kinds, applied in request order.
const (
	MembersAdd MemberOpKind = iota + 1
	MembersRemove
	MembersReplace
)

// MemberOp is one ordered change to a group's member set.
type MemberOp struct {
	Kind MemberOpKind
	IDs  []string
}

// GroupChange is what a group PATCH changes.
type GroupChange struct {
	DisplayName *string
	ExternalID  *string
	Members     []MemberOp
}

// ParseGroupPatch folds a group PatchOp into a GroupChange, keeping member
// operations in order because add then remove is not remove then add.
func ParseGroupPatch(body []byte) (GroupChange, error) {
	ops, err := decodePatch(body)
	if err != nil {
		return GroupChange{}, err
	}
	var c GroupChange
	for _, op := range ops {
		path := strings.TrimSpace(op.Path)
		lower := strings.ToLower(path)
		switch {
		case path == "":
			if op.Op == "remove" {
				return GroupChange{}, invalidPatch("remove needs a path")
			}
			if err := c.applyObject(op); err != nil {
				return GroupChange{}, err
			}
		case lower == "displayname" || lower == "externalid":
			var s string
			if op.Op == "remove" || json.Unmarshal(op.Value, &s) != nil {
				return GroupChange{}, invalidPatch("%s must be a string", path)
			}
			s = strings.TrimSpace(s)
			if lower == "displayname" {
				c.DisplayName = &s
			} else {
				c.ExternalID = &s
			}
		case lower == "members":
			ids, err := refIDs(op.Value, op.Op == "remove")
			if err != nil {
				return GroupChange{}, err
			}
			c.Members = append(c.Members, memberOp(op.Op, ids))
		case strings.HasPrefix(lower, "members[") && strings.HasSuffix(path, "]") && op.Op == "remove":
			f, err := ParseFilter(path[len("members["):len(path)-1], "value")
			if err != nil || f == nil {
				return GroupChange{}, invalidPatch("unsupported members filter")
			}
			c.Members = append(c.Members, MemberOp{Kind: MembersRemove, IDs: []string{f.Value}})
		default:
			return GroupChange{}, invalidPatch("unsupported path %q", path)
		}
	}
	return c, nil
}

func (c *GroupChange) applyObject(op PatchOperation) error {
	var obj struct {
		DisplayName *string         `json:"displayName"`
		ExternalID  *string         `json:"externalId"`
		Members     json.RawMessage `json:"members"`
	}
	if json.Unmarshal(op.Value, &obj) != nil {
		return invalidPatch("value must be an object when path is empty")
	}
	if obj.DisplayName != nil {
		s := strings.TrimSpace(*obj.DisplayName)
		c.DisplayName = &s
	}
	if obj.ExternalID != nil {
		s := strings.TrimSpace(*obj.ExternalID)
		c.ExternalID = &s
	}
	if len(obj.Members) > 0 {
		ids, err := refIDs(obj.Members, false)
		if err != nil {
			return err
		}
		c.Members = append(c.Members, memberOp(op.Op, ids))
	}
	return nil
}

func memberOp(op string, ids []string) MemberOp {
	switch op {
	case "add":
		return MemberOp{Kind: MembersAdd, IDs: ids}
	case "remove":
		if ids == nil {
			return MemberOp{Kind: MembersReplace, IDs: []string{}}
		}
		return MemberOp{Kind: MembersRemove, IDs: ids}
	}
	return MemberOp{Kind: MembersReplace, IDs: ids}
}

// refIDs reads a members value; a remove without a value means "remove every member".
func refIDs(raw json.RawMessage, optional bool) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if optional {
			return nil, nil
		}
		return nil, invalidPatch("members needs a value")
	}
	var refs []Ref
	if json.Unmarshal(raw, &refs) != nil {
		return nil, invalidPatch("members must be a list of {value}")
	}
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		if strings.TrimSpace(r.Value) == "" {
			return nil, invalidPatch("member value is empty")
		}
		ids = append(ids, strings.TrimSpace(r.Value))
	}
	return ids, nil
}
