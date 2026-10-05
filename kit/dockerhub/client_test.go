package dockerhub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), BaseURL: srv.URL}
}

func TestClient_SearchRepositories(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		query   string
		want    []Repository
		wantErr bool
	}{
		{
			name: "success",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/search/repositories/" {
					t.Errorf("path = %q, want /v2/search/repositories/", r.URL.Path)
				}
				if got := r.URL.Query().Get("query"); got != "postgres" {
					t.Errorf("query param = %q, want postgres", got)
				}
				if got := r.URL.Query().Get("page_size"); got != "25" {
					t.Errorf("page_size param = %q, want 25", got)
				}
				_, _ = w.Write([]byte(`{"count":2,"results":[
					{"repo_name":"postgres","short_description":"The PostgreSQL database","star_count":12000,"is_official":true,"is_automated":false},
					{"repo_name":"bitnami/postgresql","short_description":"Bitnami postgres","star_count":300,"is_official":false,"is_automated":true}
				]}`))
			},
			query: "postgres",
			want: []Repository{
				{Name: "postgres", ShortDescription: "The PostgreSQL database", StarCount: 12000, IsOfficial: true, IsAutomated: false},
				{Name: "bitnami/postgresql", ShortDescription: "Bitnami postgres", StarCount: 300, IsOfficial: false, IsAutomated: true},
			},
		},
		{
			name: "no results",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"count":0,"results":[]}`))
			},
			query: "zzzznosuchimage",
			want:  nil,
		},
		{
			name: "rate limited",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			query:   "postgres",
			wantErr: true,
		},
		{
			name: "invalid json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`not json`))
			},
			query:   "postgres",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.handler)
			got, err := c.SearchRepositories(context.Background(), tt.query, 25)
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("results = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("result[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestClient_ListTags(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		namespace  string
		repository string
		want       []Tag
		wantErr    bool
		wantNotFnd bool
	}{
		{
			name: "explicit namespace",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/repositories/bitnami/postgresql/tags" {
					t.Errorf("path = %q, want /v2/repositories/bitnami/postgresql/tags", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"count":2,"results":[{"name":"16"},{"name":"latest"}]}`))
			},
			namespace:  "bitnami",
			repository: "postgresql",
			want:       []Tag{{Name: "16"}, {Name: "latest"}},
		},
		{
			name: "empty namespace defaults to library",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/repositories/library/postgres/tags" {
					t.Errorf("path = %q, want /v2/repositories/library/postgres/tags", r.URL.Path)
				}
				_, _ = w.Write([]byte(`{"count":1,"results":[{"name":"16"}]}`))
			},
			namespace:  "",
			repository: "postgres",
			want:       []Tag{{Name: "16"}},
		},
		{
			name: "repository not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			namespace:  "library",
			repository: "ghost",
			wantErr:    true,
			wantNotFnd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.handler)
			got, err := c.ListTags(context.Background(), tt.namespace, tt.repository, 25)
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want an error")
				}
				if tt.wantNotFnd && !errors.Is(err, ErrNotFound) {
					t.Errorf("error = %v, want ErrNotFound", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("tags = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tag[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestClient_UnreachableServer(t *testing.T) {
	c := NewClient()
	c.BaseURL = "http://127.0.0.1:1"
	_, err := c.SearchRepositories(context.Background(), "postgres", 25)
	if err == nil {
		t.Fatal("error = nil, want an error dialing an unreachable server")
	}
}
