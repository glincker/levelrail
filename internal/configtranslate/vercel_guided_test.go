package configtranslate

import "testing"

func TestSummarizeVercel(t *testing.T) {
	files := map[string][]byte{
		"vercel.json":  []byte(`{"regions":["iad1"],"redirects":[{"source":"/a","destination":"/b"}],"build":{"env":{"X":"1"}}}`),
		"package.json": []byte(`{"scripts":{"build":"next build"},"dependencies":{"next":"15"}}`),
	}
	s := SummarizeVercel(files)
	if s.Framework != "next" || s.Build != "next build" || s.Redirects != 1 || len(s.Regions) != 1 || s.EnvKeys[0] != "X" {
		t.Fatalf("unexpected summary %+v", s)
	}
	if !IsBuildTimeEnvKey("NEXT_PUBLIC_A") || !IsBuildTimeEnvKey("VITE_B") || IsBuildTimeEnvKey("DB_URL") {
		t.Fatal("build time key detection wrong")
	}
	if len(VercelCutoverChecklist()) == 0 {
		t.Fatal("empty checklist")
	}
}
