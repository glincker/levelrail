package build

import (
	"errors"
	"testing"

	bkclient "github.com/moby/buildkit/client"
)

func TestCacheConfig_Empty(t *testing.T) {
	tests := []struct {
		name string
		cfg  CacheConfig
		want bool
	}{
		{name: "zero value", cfg: CacheConfig{}, want: true},
		{name: "dir set", cfg: CacheConfig{Dir: "/tmp/cache"}, want: false},
		{name: "registry ref set", cfg: CacheConfig{RegistryRef: "example.com/cache:tag"}, want: false},
		{name: "insecure alone still counts as empty (no ref)", cfg: CacheConfig{RegistryInsecure: true}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.empty(); got != tt.want {
				t.Errorf("empty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCacheConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CacheConfig
		wantErr error
	}{
		{name: "zero value is valid", cfg: CacheConfig{}},
		{name: "dir only is valid", cfg: CacheConfig{Dir: "/tmp/cache"}},
		{name: "registry ref only is valid", cfg: CacheConfig{RegistryRef: "example.com/cache:tag"}},
		{
			name: "registry ref with insecure is valid",
			cfg:  CacheConfig{RegistryRef: "example.com/cache:tag", RegistryInsecure: true},
		},
		{
			name:    "insecure without a ref is rejected",
			cfg:     CacheConfig{RegistryInsecure: true},
			wantErr: ErrCacheRegistryRefRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validate() unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validate() err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCacheConfig_Entries(t *testing.T) {
	tests := []struct {
		name        string
		cfg         CacheConfig
		wantErr     error
		wantImports []bkclient.CacheOptionsEntry
		wantExports []bkclient.CacheOptionsEntry
	}{
		{
			name: "zero value produces no entries",
		},
		{
			name: "local backend only",
			cfg:  CacheConfig{Dir: "/tmp/cache"},
			wantImports: []bkclient.CacheOptionsEntry{
				{Type: "local", Attrs: map[string]string{"src": "/tmp/cache"}},
			},
			wantExports: []bkclient.CacheOptionsEntry{
				{Type: "local", Attrs: map[string]string{"dest": "/tmp/cache", "mode": "max"}},
			},
		},
		{
			name: "registry backend only",
			cfg:  CacheConfig{RegistryRef: "example.com/cache:tag"},
			wantImports: []bkclient.CacheOptionsEntry{
				{Type: "registry", Attrs: map[string]string{"ref": "example.com/cache:tag", "mode": "max"}},
			},
			wantExports: []bkclient.CacheOptionsEntry{
				{Type: "registry", Attrs: map[string]string{"ref": "example.com/cache:tag", "mode": "max"}},
			},
		},
		{
			name: "registry backend, insecure",
			cfg:  CacheConfig{RegistryRef: "example.com/cache:tag", RegistryInsecure: true},
			wantImports: []bkclient.CacheOptionsEntry{
				{Type: "registry", Attrs: map[string]string{"ref": "example.com/cache:tag", "mode": "max", "insecure": "true"}},
			},
			wantExports: []bkclient.CacheOptionsEntry{
				{Type: "registry", Attrs: map[string]string{"ref": "example.com/cache:tag", "mode": "max", "insecure": "true"}},
			},
		},
		{
			name: "both backends configured together",
			cfg:  CacheConfig{Dir: "/tmp/cache", RegistryRef: "example.com/cache:tag"},
			wantImports: []bkclient.CacheOptionsEntry{
				{Type: "local", Attrs: map[string]string{"src": "/tmp/cache"}},
				{Type: "registry", Attrs: map[string]string{"ref": "example.com/cache:tag", "mode": "max"}},
			},
			wantExports: []bkclient.CacheOptionsEntry{
				{Type: "local", Attrs: map[string]string{"dest": "/tmp/cache", "mode": "max"}},
				{Type: "registry", Attrs: map[string]string{"ref": "example.com/cache:tag", "mode": "max"}},
			},
		},
		{
			name:    "invalid config fails before building any entries",
			cfg:     CacheConfig{RegistryInsecure: true},
			wantErr: ErrCacheRegistryRefRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imports, exports, err := tt.cfg.entries()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("entries() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("entries() unexpected error: %v", err)
			}
			if len(imports) != len(tt.wantImports) {
				t.Fatalf("imports = %+v, want %+v", imports, tt.wantImports)
			}
			for i := range imports {
				if imports[i].Type != tt.wantImports[i].Type {
					t.Errorf("imports[%d].Type = %q, want %q", i, imports[i].Type, tt.wantImports[i].Type)
				}
				for k, v := range tt.wantImports[i].Attrs {
					if imports[i].Attrs[k] != v {
						t.Errorf("imports[%d].Attrs[%q] = %q, want %q", i, k, imports[i].Attrs[k], v)
					}
				}
			}
			if len(exports) != len(tt.wantExports) {
				t.Fatalf("exports = %+v, want %+v", exports, tt.wantExports)
			}
			for i := range exports {
				if exports[i].Type != tt.wantExports[i].Type {
					t.Errorf("exports[%d].Type = %q, want %q", i, exports[i].Type, tt.wantExports[i].Type)
				}
				for k, v := range tt.wantExports[i].Attrs {
					if exports[i].Attrs[k] != v {
						t.Errorf("exports[%d].Attrs[%q] = %q, want %q", i, k, exports[i].Attrs[k], v)
					}
				}
			}
		})
	}
}

func TestCacheConfig_String(t *testing.T) {
	tests := []struct {
		name string
		cfg  CacheConfig
		want string
	}{
		{name: "disabled", cfg: CacheConfig{}, want: "disabled"},
		{name: "dir only", cfg: CacheConfig{Dir: "/tmp/cache"}, want: "dir=/tmp/cache"},
		{name: "registry only", cfg: CacheConfig{RegistryRef: "example.com/cache:tag"}, want: "registry=example.com/cache:tag"},
		{
			name: "both",
			cfg:  CacheConfig{Dir: "/tmp/cache", RegistryRef: "example.com/cache:tag"},
			want: "dir=/tmp/cache registry=example.com/cache:tag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
