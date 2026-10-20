package edgefw

import (
	"net/netip"
	"sort"
	"strings"
	"time"
)

type tkey struct {
	s string
	a netip.Addr
}

type topEntry struct{ n uint32 }

// sketch is a Space-Saving counter: it holds at most capacity keys and, when
// full, hands the least counted key's slot to the newcomer.
type sketch struct {
	start int64
	m     map[tkey]*topEntry
}

func (k *sketch) add(key tkey, start int64, capacity int) {
	if k.m == nil || k.start != start {
		k.m, k.start = make(map[tkey]*topEntry, capacity), start
	}
	if e, ok := k.m[key]; ok {
		e.n++
		return
	}
	key.s = strings.Clone(key.s)
	if len(k.m) < capacity {
		k.m[key] = &topEntry{n: 1}
		return
	}
	var lowKey tkey
	var low *topEntry
	for kk, e := range k.m {
		if low == nil || e.n < low.n {
			lowKey, low = kk, e
		}
	}
	delete(k.m, lowKey)
	low.n++
	k.m[key] = low
}

const dimCount = 5

type topSet struct {
	capacity int
	dims     [dimCount][sparkSlots]sketch
}

func newTopSet(capacity int) *topSet { return &topSet{capacity: capacity} }

func dimIndex(d Dimension) int {
	switch d {
	case DimIP:
		return 0
	case DimDeniedIP:
		return 1
	case DimUA:
		return 2
	case DimPath:
		return 3
	case DimHost:
		return 4
	}
	return -1
}

func (t *topSet) add(o Observation) {
	start := o.At.Unix() / slotSeconds
	slot := int(start % sparkSlots)
	if o.IP.IsValid() {
		t.dims[0][slot].add(tkey{a: o.IP}, start, t.capacity)
		if o.Outcome == OutcomeDenied || o.Outcome == OutcomeChallenged || o.Outcome == OutcomeRateLimited {
			t.dims[1][slot].add(tkey{a: o.IP}, start, t.capacity)
		}
	}
	ua := o.UA
	if ua == "" {
		ua = "(none)"
	}
	t.dims[2][slot].add(tkey{s: clip(ua, 120)}, start, t.capacity)
	t.dims[3][slot].add(tkey{s: normalizePath(o.Path)}, start, t.capacity)
	if o.Host != "" {
		t.dims[4][slot].add(tkey{s: o.Host}, start, t.capacity)
	}
}

func (t *topSet) rows(d Dimension, now time.Time, n int) []TopRow {
	di := dimIndex(d)
	if di < 0 {
		return []TopRow{}
	}
	cur := now.Unix() / slotSeconds
	merged := map[tkey]*[sparkSlots]uint32{}
	for back := int64(0); back < sparkSlots; back++ {
		start := cur - back
		sk := &t.dims[di][start%sparkSlots]
		if sk.m == nil || sk.start != start {
			continue
		}
		for k, e := range sk.m {
			arr := merged[k]
			if arr == nil {
				arr = &[sparkSlots]uint32{}
				merged[k] = arr
			}
			arr[sparkSlots-1-int(back)] += e.n
		}
	}
	rows := make([]TopRow, 0, len(merged))
	for k, arr := range merged {
		var total uint64
		for _, v := range arr {
			total += uint64(v)
		}
		key := k.s
		if k.a.IsValid() {
			key = k.a.String()
		}
		rows = append(rows, TopRow{Key: key, Count: total, Spark: *arr})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Key < rows[j].Key
	})
	if n > 0 && len(rows) > n {
		rows = rows[:n]
	}
	return rows
}
