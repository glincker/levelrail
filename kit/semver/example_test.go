package semver_test

import (
	"fmt"

	"github.com/GLINCKER/levelrail/kit/semver"
)

func ExampleCompare() {
	fmt.Println(semver.Compare("v1.2.3", "v1.10.0"))
	fmt.Println(semver.Compare("0.4.0-beta.2", "0.4.0"))
	fmt.Println(semver.Compare("dev", "v1.0.0"))
	// Output:
	// -1 true
	// -1 true
	// 0 false
}
