package build

import (
	"strings"
	"testing"
)

const next14Log = `   ▲ Next.js 14.2.5

   Creating an optimized production build ...
 ⚠ Compiled with warnings

./src/lib/x.ts
Warning: Unused variable foo

 ✓ Compiled successfully
Route (app)                              Size     First Load JS
┌ ○ /                                    5.4 kB         92.5 kB
├ ○ /_not-found                          871 B          88 kB
├ ƒ /api/hello                           0 B                0 B
├ ● /blog/[slug]                         1.2 kB         90 kB
├   ├ /blog/first
├   └ [+2 more paths]
└ ○ /about                               137 B          87.3 kB
+ First Load JS shared by all            87.1 kB
  ├ chunks/117-abc.js                    31.6 kB
  └ other shared chunks (total)          1.9 kB


○  (Static)   prerendered as static content
●  (SSG)      prerendered as static HTML (uses getStaticProps)
ƒ  (Dynamic)  server-rendered on demand
`

const next15ISRLog = `Route (app)                                 Size  First Load JS  Revalidate  Expire
┌ ○ /                                      123 B         101 kB
├ ○ /pricing                             1.5 kB         103 kB          1h      1y
└ ƒ /dash                                2.1 MB         2.2 MB

Route (pages)                               Size  First Load JS
├ ○ /legacy                              300 B          80 kB
┌ λ /old/[id]                            1 kB          81 kB
+ First Load JS shared by all             100 kB
`

const next16Log = `▲ Next.js 16.0.1 (Turbopack)

Route (app)
┌ ○ /
├ ○ /_not-found
├ ◐ /shop
└ ƒ /api/items

○  (Static)   prerendered as static content
◐  (Partial Prerender)  prerendered as static HTML with dynamic server-streamed content
ƒ  (Dynamic)  server-rendered on demand
`

func TestParseNextReport(t *testing.T) {
	ansi := "\x1b[1mRoute (app)\x1b[22m   Size\n\x1b[32m┌ ○ /\x1b[39m   5 kB   90 kB\n"
	prefixed := "#14 62.43 Route (app)   Size  First Load JS\n#14 62.43 ┌ ○ /   5.4 kB   92.5 kB\n#14 62.43 + First Load JS shared by all   87.1 kB\n"
	f := func(v float64) *float64 { return &v }
	tests := []struct {
		name      string
		log       string
		detected  bool
		routes    []NextRoute
		shared    *float64
		warnCount int
	}{
		{"next14", next14Log, true, []NextRoute{
			{"/", NextKindStatic, 5.4, 92.5},
			{"/_not-found", NextKindStatic, 0.87, 88},
			{"/api/hello", NextKindDynamic, 0, 0},
			{"/blog/[slug]", NextKindStatic, 1.2, 90},
			{"/about", NextKindStatic, 0.14, 87.3},
		}, f(87.1), 2},
		{"next15 isr and pages", next15ISRLog, true, []NextRoute{
			{"/", NextKindStatic, 0.12, 101},
			{"/pricing", NextKindISR, 1.5, 103},
			{"/dash", NextKindDynamic, 2100, 2200},
			{"/legacy", NextKindStatic, 0.3, 80},
			{"/old/[id]", NextKindDynamic, 1, 81},
		}, f(100), 0},
		{"next16 no columns", next16Log, true, []NextRoute{
			{"/", NextKindStatic, 0, 0},
			{"/_not-found", NextKindStatic, 0, 0},
			{"/shop", NextKindUnknown, 0, 0},
			{"/api/items", NextKindDynamic, 0, 0},
		}, nil, 0},
		{"ansi stripped", ansi, true, []NextRoute{{"/", NextKindStatic, 5, 90}}, nil, 0},
		{"buildkit prefix", prefixed, true, []NextRoute{{"/", NextKindStatic, 5.4, 92.5}}, f(87.1), 0},
		{"not next", "go build ./...\nok\n", false, []NextRoute{}, nil, 0},
		{"empty", "", false, []NextRoute{}, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseNextReport(strings.Split(tt.log, "\n"))
			if got.Detected != tt.detected {
				t.Fatalf("detected = %v", got.Detected)
			}
			if len(got.Routes) != len(tt.routes) {
				t.Fatalf("routes = %+v", got.Routes)
			}
			for i, r := range tt.routes {
				if got.Routes[i] != r {
					t.Errorf("route %d = %+v, want %+v", i, got.Routes[i], r)
				}
			}
			if (got.SharedFirstLoad == nil) != (tt.shared == nil) || (tt.shared != nil && *got.SharedFirstLoad != *tt.shared) {
				t.Errorf("shared = %v", got.SharedFirstLoad)
			}
			if got.Warnings == nil || len(got.Warnings) != tt.warnCount {
				t.Errorf("warnings = %v", got.Warnings)
			}
		})
	}
}
