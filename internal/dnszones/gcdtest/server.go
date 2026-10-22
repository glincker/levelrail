// Package gcdtest is an in-memory fake of the Cloud DNS v1 REST API for tests.
package gcdtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// RRSet is one stored record set.
type RRSet struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	RRDatas []string `json:"rrdatas"`
}

type zone struct {
	Name        string   `json:"name"`
	DNSName     string   `json:"dnsName"`
	NameServers []string `json:"nameServers"`
	sets        []RRSet
}

// Server serves one project's managed zones.
type Server struct {
	*httptest.Server
	mu      sync.Mutex
	project string
	zones   map[string]*zone
	// Calls records "METHOD path" for every request.
	Calls []string
}

// New starts a fake for project, seeded with one zone per dnsName ("example.com.").
func New(project string, dnsNames ...string) *Server {
	s := &Server{project: project, zones: map[string]*zone{}}
	for _, n := range dnsNames {
		s.addZone(strings.ReplaceAll(strings.TrimSuffix(n, "."), ".", "-"), n)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	return s
}

func (s *Server) addZone(id, dnsName string) *zone {
	z := &zone{Name: id, DNSName: dnsName, NameServers: []string{"ns-cloud-a1.googledomains.com."}}
	z.sets = []RRSet{
		{Name: dnsName, Type: "NS", TTL: 21600, RRDatas: z.NameServers},
		{Name: dnsName, Type: "SOA", TTL: 21600, RRDatas: []string{"ns-cloud-a1.googledomains.com. cloud-dns-hostmaster.google.com. 1 21600 3600 259200 300"}},
	}
	s.zones[id] = z
	return z
}

// Sets returns a copy of a zone's record sets.
func (s *Server) Sets(zoneID string) []RRSet {
	s.mu.Lock()
	defer s.mu.Unlock()
	if z, ok := s.zones[zoneID]; ok {
		return append([]RRSet(nil), z.sets...)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": 404, "message": "not found"}})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, r.Method+" "+r.URL.Path)
	prefix := "/projects/" + s.project + "/managedZones"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		notFound(w)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/"), "/")
	if parts[0] == "" {
		s.zoneCollection(w, r)
		return
	}
	z, ok := s.zones[parts[0]]
	if !ok {
		notFound(w)
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, z)
	case len(parts) == 1 && r.Method == http.MethodDelete:
		delete(s.zones, z.Name)
		writeJSON(w, http.StatusOK, map[string]any{})
	case len(parts) == 2 && parts[1] == "rrsets" && r.Method == http.MethodGet:
		name, typ := r.URL.Query().Get("name"), r.URL.Query().Get("type")
		out := []RRSet{}
		for _, rs := range z.sets {
			if (name == "" || rs.Name == name) && (typ == "" || rs.Type == typ) {
				out = append(out, rs)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"rrsets": out})
	case len(parts) == 2 && parts[1] == "changes" && r.Method == http.MethodPost:
		s.applyChange(w, r, z)
	default:
		notFound(w)
	}
}

func (s *Server) zoneCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		out := []*zone{}
		for _, z := range s.zones {
			out = append(out, z)
		}
		writeJSON(w, http.StatusOK, map[string]any{"managedZones": out})
	case http.MethodPost:
		var in zone
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "bad body"}})
			return
		}
		writeJSON(w, http.StatusOK, s.addZone(in.Name, in.DNSName))
	default:
		notFound(w)
	}
}

func (s *Server) applyChange(w http.ResponseWriter, r *http.Request, z *zone) {
	var in struct {
		Additions []RRSet `json:"additions"`
		Deletions []RRSet `json:"deletions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "bad body"}})
		return
	}
	for _, d := range in.Deletions {
		idx := -1
		for i, rs := range z.sets {
			if rs.Name == d.Name && rs.Type == d.Type {
				idx = i
			}
		}
		if idx < 0 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "deletion target does not exist"}})
			return
		}
		z.sets = append(z.sets[:idx], z.sets[idx+1:]...)
	}
	for _, a := range in.Additions {
		for _, rs := range z.sets {
			if rs.Name == a.Name && rs.Type == a.Type {
				writeJSON(w, http.StatusConflict, map[string]any{"error": map[string]any{"message": "record already exists"}})
				return
			}
		}
		z.sets = append(z.sets, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "done"})
}
