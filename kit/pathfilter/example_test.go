package pathfilter_test

import (
	"fmt"

	"github.com/GLINCKER/levelrail/kit/pathfilter"
)

func ExampleFilter_Apply() {
	f := pathfilter.Filter{
		Paths:  []string{"services/api/**"},
		Ignore: []string{"**/*.md"},
	}
	fmt.Println(f.Apply([]string{"services/api/main.go"}).Run)
	fmt.Println(f.Apply([]string{"services/web/index.ts"}).Reason)
	fmt.Println(f.Apply([]string{"services/api/README.md"}).Reason)
	// Output:
	// true
	// skipped: no changed path matched paths
	// skipped: every changed path matched paths_ignore
}

func ExampleMatch() {
	fmt.Println(pathfilter.Match("src/**/*.{go,ts}", "src/a/b/c.go"))
	fmt.Println(pathfilter.Match("docs/", "docs/guide/intro.md"))
	// Output:
	// true
	// true
}
