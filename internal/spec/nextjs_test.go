package spec

import "testing"

func TestNextCacheValidation(t *testing.T) {
	cases := []struct {
		name    string
		next    *Next
		wantErr bool
	}{
		{"absent", nil, false},
		{"empty", &Next{}, false},
		{"none", &Next{Cache: "none"}, false},
		{"redis", &Next{Cache: "redis"}, false},
		{"unknown", &Next{Cache: "memcached"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &Service{Next: tc.next}
			if err := svc.validateNext("web"); (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestNextCacheParsesFromYAML(t *testing.T) {
	doc := "version: 1\nservices:\n  web:\n    image: nginx:1\n    port: 80\n    next:\n      cache: redis\n"
	s, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Services["web"].Next.UsesRedisCache() {
		t.Fatal("next.cache redis not parsed")
	}
	bad := "version: 1\nservices:\n  web:\n    image: nginx:1\n    port: 80\n    next:\n      cache: bogus\n"
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("schema should reject unknown next.cache")
	}
}
