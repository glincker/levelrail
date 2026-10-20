package stackdetect_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

func ExampleDetect() {
	dir, err := os.MkdirTemp("", "app")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	pkg := `{"dependencies":{"express":"4"},"scripts":{"start":"node server.js"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o600); err != nil {
		panic(err)
	}

	s, err := stackdetect.Detect(dir)
	if err != nil {
		panic(err)
	}
	fmt.Println(s.Label, s.Build, s.Port, s.Detected())
	// Output:
	// Node.js (Express) railpack 3000 true
}
