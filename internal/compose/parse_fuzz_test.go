package compose

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte("services:\n  web:\n    image: nginx\n    ports: [\"80:80\"]\n"))
	f.Add([]byte("services:\n  a:\n    image: ${X:-y}\n    environment:\n      - K=${V?err}\n"))
	f.Add([]byte("x-a: &a {image: b}\nservices:\n  s: *a\n"))
	f.Add([]byte("a: &a [*a]\n"))
	f.Add([]byte(""))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = Parse(data)
	})
}
