package cronexpr_test

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

func ExampleSchedule_Next() {
	s, err := cronexpr.Parse("0 3 * * *")
	if err != nil {
		panic(err)
	}
	after := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	fmt.Println(s.Next(after).Format(time.RFC3339))
	// Output:
	// 2026-01-11T03:00:00Z
}

func ExampleNextInLocation() {
	s, err := cronexpr.Parse("30 9 * * 1-5")
	if err != nil {
		panic(err)
	}
	loc := time.FixedZone("UTC+2", 2*60*60)
	after := time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC)
	fmt.Println(cronexpr.NextInLocation(s, after, loc).Format(time.RFC3339))
	// Output:
	// 2026-01-12T09:30:00+02:00
}
