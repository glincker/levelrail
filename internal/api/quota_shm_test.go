package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestServiceReservationCountsShm(t *testing.T) {
	cases := []struct {
		res  store.ServiceResources
		want int64
	}{
		{store.ServiceResources{ShmSizeBytes: 1 << 30}, 2 << 30},
		{store.ServiceResources{MemoryBytes: 512 << 20, ShmSizeBytes: 1 << 30}, 1 << 30},
		{store.ServiceResources{MemoryBytes: 512 << 20}, 1 << 30},
	}
	for _, c := range cases {
		if got := serviceReservation(&c.res, 2).MemoryBytes; got != c.want {
			t.Errorf("serviceReservation(%+v, 2).MemoryBytes = %d, want %d", c.res, got, c.want)
		}
	}
}
