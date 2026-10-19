package spec

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte("version: 1\nservices:\n  web:\n    port: 3000\n"))
	f.Add([]byte("version: 1\nservices: {}\n"))
	f.Add([]byte("a: &a [*a]\n"))
	f.Add([]byte("\x00\xff"))
	f.Add([]byte(""))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = Parse(data)
	})
}
