package diskspace_test

import (
	"fmt"

	"github.com/GLINCKER/levelrail/kit/diskspace"
)

func ExampleHumanBytes() {
	fmt.Println(diskspace.HumanBytes(1536))
	fmt.Println(diskspace.HumanBytes(5 << 30))
	// Output:
	// 1.5 KiB
	// 5.0 GiB
}
