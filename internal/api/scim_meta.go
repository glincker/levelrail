package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/scim"
	"github.com/GLINCKER/levelrail/internal/store"
)

type scimSupported struct {
	Supported bool `json:"supported"`
}

type scimFilterSupport struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

type scimBulkSupport struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

type scimAuthScheme struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Primary     bool   `json:"primary"`
}

type scimServiceProviderConfig struct {
	Schemas               []string          `json:"schemas"`
	Patch                 scimSupported     `json:"patch"`
	Bulk                  scimBulkSupport   `json:"bulk"`
	Filter                scimFilterSupport `json:"filter"`
	ChangePassword        scimSupported     `json:"changePassword"`
	Sort                  scimSupported     `json:"sort"`
	ETag                  scimSupported     `json:"etag"`
	AuthenticationSchemes []scimAuthScheme  `json:"authenticationSchemes"`
}

type scimResourceType struct {
	Schemas  []string `json:"schemas"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Endpoint string   `json:"endpoint"`
	Schema   string   `json:"schema"`
}

type scimResourceTypeList struct {
	scim.ListHeader
	Resources []scimResourceType `json:"Resources"`
}

func (rt *Router) handleSCIMServiceProviderConfig(w http.ResponseWriter, _ *http.Request, _ store.SCIMToken) {
	writeSCIM(w, http.StatusOK, scimServiceProviderConfig{
		Schemas: []string{scim.SchemaSPConfig},
		Patch:   scimSupported{Supported: true},
		Filter:  scimFilterSupport{Supported: true, MaxResults: scimMaxPage},
		AuthenticationSchemes: []scimAuthScheme{{
			Type: "oauthbearertoken", Name: "Bearer token", Primary: true,
			Description: "A SCIM token created under Settings, Single sign-on",
		}},
	})
}

func (rt *Router) handleSCIMResourceTypes(w http.ResponseWriter, _ *http.Request, _ store.SCIMToken) {
	types := []scimResourceType{
		{Schemas: []string{scim.SchemaResourceType}, ID: "User", Name: "User", Endpoint: "/Users", Schema: scim.SchemaUser},
		{Schemas: []string{scim.SchemaResourceType}, ID: "Group", Name: "Group", Endpoint: "/Groups", Schema: scim.SchemaGroup},
	}
	writeSCIM(w, http.StatusOK, scimResourceTypeList{ListHeader: scim.NewListHeader(len(types), len(types), 1), Resources: types})
}
