package quota

import (
	"slices"
	"testing"
)

func TestCheck(t *testing.T) {
	limits := Amounts{Apps: 3, MemoryBytes: 1000}
	tests := []struct {
		name string
		used Amounts
		req  Amounts
		want []string
	}{
		{"under limit", Amounts{Apps: 1}, Amounts{Apps: 1}, nil},
		{"exactly at limit is allowed", Amounts{Apps: 2}, Amounts{Apps: 1}, nil},
		{"one over", Amounts{Apps: 3}, Amounts{Apps: 1}, []string{ResourceApps}},
		{"memory over", Amounts{MemoryBytes: 900}, Amounts{MemoryBytes: 101}, []string{ResourceMemory}},
		{"both over", Amounts{Apps: 3, MemoryBytes: 1000}, Amounts{Apps: 1, MemoryBytes: 1}, []string{ResourceApps, ResourceMemory}},
		{"zero request never violates", Amounts{Apps: 9, MemoryBytes: 5000}, Amounts{}, nil},
		{"unlimited dimension ignored", Amounts{Databases: 99}, Amounts{Databases: 1}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, v := range Check(limits, tc.used, tc.req) {
				got = append(got, v.Resource)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestMergeAndPercent(t *testing.T) {
	got := Merge(Amounts{Apps: 2}, Amounts{Apps: 10, Databases: 4})
	if got.Apps != 2 || got.Databases != 4 {
		t.Fatalf("merge: %+v", got)
	}
	if Percent(5, 0) != -1 || Percent(8, 10) != 80 || Percent(15, 10) != 150 {
		t.Fatal("percent")
	}
}
