// Minimal fixture for the internal/build Railpack integration: enough for
// Railpack's golang provider to detect a Go app and generate a build
// plan, and, once built, enough to prove the resulting image actually
// runs (see TestClient_BuildRailpack_Live_Go in railpack_test.go).
package main

import "fmt"

func main() {
	fmt.Println("levelrail railpack go fixture")
}
