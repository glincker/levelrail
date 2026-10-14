package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/samlsso"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envSAMLRatePerMinute     = "APP_SAML_RATE_PER_MINUTE"
	defaultSAMLRatePerMinute = 30
	maxSAMLNameLen           = 64
	maxSAMLAttributeLen      = 256
)

var samlDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

type samlCertificateInfo struct {
	Subject  string `json:"subject"`
	NotAfter string `json:"not_after"`
}

type samlConnectionResource struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Enabled         bool                  `json:"enabled"`
	Configured      bool                  `json:"configured"`
	IDPEntityID     string                `json:"idp_entity_id"`
	IDPSSOURL       string                `json:"idp_sso_url"`
	IDPCertificates string                `json:"idp_certificates"`
	IDPMetadataURL  string                `json:"idp_metadata_url"`
	Certificates    []samlCertificateInfo `json:"certificates"`
	EmailAttribute  string                `json:"email_attribute"`
	NameAttribute   string                `json:"name_attribute"`
	GroupsAttribute string                `json:"groups_attribute"`
	AllowedDomains  []string              `json:"allowed_domains"`
	JITProvisioning bool                  `json:"jit_provisioning"`
	DefaultRoleID   string                `json:"default_role_id"`
	SyncRoles       bool                  `json:"sync_roles"`
	SPEntityID      string                `json:"sp_entity_id"`
	SPACSURL        string                `json:"sp_acs_url"`
	SPMetadataURL   string                `json:"sp_metadata_url"`
	CreatedAt       string                `json:"created_at"`
	UpdatedAt       string                `json:"updated_at"`
}

type samlConnectionRequest struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	IDPEntityID     string   `json:"idp_entity_id"`
	IDPSSOURL       string   `json:"idp_sso_url"`
	IDPCertificates string   `json:"idp_certificates"`
	IDPMetadataURL  string   `json:"idp_metadata_url"`
	EmailAttribute  string   `json:"email_attribute"`
	NameAttribute   string   `json:"name_attribute"`
	GroupsAttribute string   `json:"groups_attribute"`
	AllowedDomains  []string `json:"allowed_domains"`
	JITProvisioning bool     `json:"jit_provisioning"`
	DefaultRoleID   string   `json:"default_role_id"`
	SyncRoles       bool     `json:"sync_roles"`
}

type samlMetadataImportRequest struct {
	URL string `json:"url"`
	XML string `json:"xml"`
}

func (rt *Router) samlResource(r *http.Request, c store.SAMLConnection) samlConnectionResource {
	res := samlConnectionResource{
		ID: c.ID, Name: c.Name, Enabled: c.Enabled, Configured: samlConfigured(c),
		IDPEntityID: c.IDPEntityID, IDPSSOURL: c.IDPSSOURL, IDPCertificates: c.IDPCertificates, IDPMetadataURL: c.IDPMetadataURL,
		Certificates: []samlCertificateInfo{}, EmailAttribute: c.EmailAttribute, NameAttribute: c.NameAttribute,
		GroupsAttribute: c.GroupsAttribute, AllowedDomains: nonNilStrings(c.AllowedDomains), JITProvisioning: c.JITProvisioning,
		DefaultRoleID: c.DefaultRoleID, SyncRoles: c.SyncRoles,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if certs, err := samlsso.ParseCertificates(c.IDPCertificates); err == nil {
		for _, cert := range certs {
			res.Certificates = append(res.Certificates, samlCertificateInfo{Subject: cert.Subject.String(), NotAfter: cert.NotAfter.UTC().Format(time.RFC3339)})
		}
	}
	if base, err := rt.samlBaseURL(r.Context()); err == nil {
		res.SPEntityID, res.SPACSURL = samlEndpoints(base, c.ID)
		res.SPMetadataURL = res.SPEntityID
	}
	return res
}

func (rt *Router) ssoReady(w http.ResponseWriter) bool {
	if rt.sso == nil {
		writeError(w, http.StatusNotImplemented, errSSOUnavailable.Error())
		return false
	}
	return true
}

func normalizeSAMLDomains(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" {
			continue
		}
		if !samlDomainRe.MatchString(d) {
			return nil, errors.New("allowed_domains: " + d + " is not a domain name")
		}
		out = append(out, d)
	}
	return out, nil
}

// applySAMLRequest validates req onto c. Enabling demands a complete IDP and
// an https dashboard URL, so a half-configured connection never reaches sign-in.
func (rt *Router) applySAMLRequest(r *http.Request, c *store.SAMLConnection, req samlConnectionRequest) (int, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > maxSAMLNameLen {
		return http.StatusBadRequest, errors.New("name is required and at most 64 characters")
	}
	for _, a := range []string{req.EmailAttribute, req.NameAttribute, req.GroupsAttribute} {
		if len(a) > maxSAMLAttributeLen {
			return http.StatusBadRequest, errors.New("attribute names are at most 256 characters")
		}
	}
	domains, err := normalizeSAMLDomains(req.AllowedDomains)
	if err != nil {
		return http.StatusBadRequest, err
	}
	idp := samlsso.IDP{EntityID: strings.TrimSpace(req.IDPEntityID), SSOURL: strings.TrimSpace(req.IDPSSOURL), CertificatesPEM: strings.TrimSpace(req.IDPCertificates)}
	if idp.CertificatesPEM != "" {
		certs, err := samlsso.ParseCertificates(idp.CertificatesPEM)
		if err != nil {
			return http.StatusBadRequest, errors.New("idp_certificates: " + err.Error())
		}
		idp.CertificatesPEM = samlsso.EncodeCertificates(certs)
	}
	if req.Enabled {
		if err := idp.Check(); err != nil {
			return http.StatusBadRequest, errors.New("cannot enable an incomplete connection: " + err.Error())
		}
		if _, err := rt.samlBaseURL(r.Context()); err != nil {
			return http.StatusConflict, err
		}
	}
	if req.IDPMetadataURL != "" && !strings.HasPrefix(req.IDPMetadataURL, "https://") {
		return http.StatusBadRequest, errors.New("idp_metadata_url must use https")
	}
	if req.DefaultRoleID != "" {
		if _, err := rt.roles.GetRole(r.Context(), req.DefaultRoleID); err != nil {
			return http.StatusBadRequest, errors.New("default_role_id does not name a stored role")
		}
	}
	c.Name, c.Enabled = name, req.Enabled
	c.IDPEntityID, c.IDPSSOURL, c.IDPCertificates, c.IDPMetadataURL = idp.EntityID, idp.SSOURL, idp.CertificatesPEM, strings.TrimSpace(req.IDPMetadataURL)
	c.EmailAttribute, c.NameAttribute, c.GroupsAttribute = strings.TrimSpace(req.EmailAttribute), strings.TrimSpace(req.NameAttribute), strings.TrimSpace(req.GroupsAttribute)
	c.AllowedDomains, c.JITProvisioning, c.DefaultRoleID, c.SyncRoles = domains, req.JITProvisioning, req.DefaultRoleID, req.SyncRoles
	return 0, nil
}

func (rt *Router) handleListSAMLConnections(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	conns, err := rt.sso.ListSAMLConnections(r.Context())
	if err != nil {
		rt.internalError(w, "api: list saml connections failed", err)
		return
	}
	out := make([]samlConnectionResource, 0, len(conns))
	for _, c := range conns {
		out = append(out, rt.samlResource(r, c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetSAMLConnection(w http.ResponseWriter, r *http.Request) {
	if c, ok := rt.loadSAMLConnection(w, r); ok {
		writeJSON(w, http.StatusOK, rt.samlResource(r, c))
	}
}

func (rt *Router) loadSAMLConnection(w http.ResponseWriter, r *http.Request) (store.SAMLConnection, bool) {
	if !rt.ssoReady(w) {
		return store.SAMLConnection{}, false
	}
	c, err := rt.sso.GetSAMLConnection(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrSAMLConnectionNotFound) {
		writeError(w, http.StatusNotFound, "saml connection not found")
		return store.SAMLConnection{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get saml connection failed", err)
		return store.SAMLConnection{}, false
	}
	return c, true
}

func (rt *Router) handleCreateSAMLConnection(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	var req samlConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, samlsso.MaxMetadataBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := randomOpaqueID("saml_")
	if err != nil {
		rt.internalError(w, "api: create saml connection id failed", err)
		return
	}
	c := store.SAMLConnection{ID: id}
	if status, err := rt.applySAMLRequest(r, &c, req); err != nil {
		writeError(w, status, err.Error())
		return
	}
	now := time.Now()
	if err := rt.sso.CreateSAMLConnection(r.Context(), c, now); err != nil {
		rt.writeSAMLStoreError(w, "create", err)
		return
	}
	c.CreatedAt, c.UpdatedAt = now, now
	writeJSON(w, http.StatusCreated, rt.samlResource(r, c))
}

func (rt *Router) handleUpdateSAMLConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	var req samlConnectionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, samlsso.MaxMetadataBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if status, err := rt.applySAMLRequest(r, &c, req); err != nil {
		writeError(w, status, err.Error())
		return
	}
	c.UpdatedAt = time.Now()
	if err := rt.sso.UpdateSAMLConnection(r.Context(), c, c.UpdatedAt); err != nil {
		rt.writeSAMLStoreError(w, "update", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.samlResource(r, c))
}

func (rt *Router) handleDeleteSAMLConnection(w http.ResponseWriter, r *http.Request) {
	if !rt.ssoReady(w) {
		return
	}
	if err := rt.sso.DeleteSAMLConnection(r.Context(), r.PathValue("id")); err != nil {
		rt.writeSAMLStoreError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleImportSAMLMetadata fills the IDP fields from metadata given as XML or
// fetched from an https URL. It never enables the connection by itself.
func (rt *Router) handleImportSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	var req samlMetadataImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*samlsso.MaxMetadataBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if (req.URL == "") == (req.XML == "") {
		writeError(w, http.StatusBadRequest, "give exactly one of url or xml")
		return
	}
	raw := []byte(req.XML)
	if req.URL != "" {
		body, err := samlsso.FetchIDPMetadata(r.Context(), nil, req.URL)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		raw = body
	}
	idp, err := samlsso.ParseIDPMetadata(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c.IDPEntityID, c.IDPSSOURL, c.IDPCertificates = idp.EntityID, idp.SSOURL, idp.CertificatesPEM
	if req.URL != "" {
		c.IDPMetadataURL = req.URL
	}
	c.UpdatedAt = time.Now()
	if err := rt.sso.UpdateSAMLConnection(r.Context(), c, c.UpdatedAt); err != nil {
		rt.writeSAMLStoreError(w, "import metadata", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.samlResource(r, c))
}

func (rt *Router) writeSAMLStoreError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrSAMLConnectionNotFound):
		writeError(w, http.StatusNotFound, "saml connection not found")
	case errors.Is(err, store.ErrSAMLConnectionNameTaken):
		writeError(w, http.StatusConflict, "a saml connection with that name already exists")
	default:
		rt.internalError(w, "api: "+op+" saml connection failed", err)
	}
}
