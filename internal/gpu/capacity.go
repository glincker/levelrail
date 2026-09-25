package gpu

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Sentinel reasons a claim does not fit a node.
var (
	ErrNoGPU          = errors.New("node reports no NVIDIA GPU")
	ErrRuntimeMissing = errors.New("the nvidia container runtime is not registered with Docker on the node")
	ErrUnknownDevice  = errors.New("GPU device not present on the node")
	ErrDeviceTaken    = errors.New("GPU device already reserved")
	ErrInsufficient   = errors.New("not enough free GPUs")
)

// Claim is one workload's GPU reservation. With no DeviceIDs, Count <= 0
// means every GPU on the node, matching how Docker's request is built.
type Claim struct {
	Name      string
	Count     int
	DeviceIDs []string
}

// All reports whether the claim takes every GPU on the node.
func (c Claim) All() bool { return len(c.DeviceIDs) == 0 && c.Count <= 0 }

// Ledger tracks which GPUs of one node are reserved. Docker does not
// enforce exclusivity, so this is scheduling accounting, not isolation.
type Ledger struct {
	info    Info
	pinned  map[int]string
	anon    int
	all     bool
	holders []string
}

// NewLedger builds the ledger for a node from the claims already on it.
func NewLedger(info Info, claims []Claim) Ledger {
	l := Ledger{info: info, pinned: map[int]string{}}
	for _, c := range claims {
		l.holders = append(l.holders, c.Name)
		switch {
		case c.All():
			l.all = true
		case len(c.DeviceIDs) > 0:
			for _, id := range c.DeviceIDs {
				if idx, ok := l.resolve(id); ok {
					l.pinned[idx] = c.Name
				}
			}
		default:
			l.anon += c.Count
		}
	}
	sort.Strings(l.holders)
	return l
}

// Total is the node's GPU count.
func (l Ledger) Total() int { return l.info.Count() }

// Reserved is how many GPUs are spoken for, never above Total.
func (l Ledger) Reserved() int {
	if l.all {
		return l.Total()
	}
	return min(l.Total(), len(l.pinned)+l.anon)
}

// Free is Total minus Reserved.
func (l Ledger) Free() int { return l.Total() - l.Reserved() }

// Holders lists the workloads holding a reservation, sorted.
func (l Ledger) Holders() []string { return l.holders }

func (l Ledger) resolve(id string) (int, bool) {
	id = strings.TrimSpace(id)
	for _, d := range l.info.Devices {
		if id == strconv.Itoa(d.Index) || (d.UUID != "" && strings.EqualFold(id, d.UUID)) {
			return d.Index, true
		}
	}
	return 0, false
}

// Fit reports why c cannot be placed on this node, or nil when it can.
func (l Ledger) Fit(c Claim) error {
	if !l.info.Present {
		return ErrNoGPU
	}
	if !l.info.RuntimeInstalled {
		return ErrRuntimeMissing
	}
	switch {
	case c.All():
		if l.Reserved() > 0 {
			return fmt.Errorf("%w: needs every GPU, %d of %d reserved by %s", ErrInsufficient, l.Reserved(), l.Total(), strings.Join(l.holders, ", "))
		}
	case len(c.DeviceIDs) > 0:
		for _, id := range c.DeviceIDs {
			idx, ok := l.resolve(id)
			if !ok {
				return fmt.Errorf("%w: %q", ErrUnknownDevice, id)
			}
			if holder, taken := l.pinned[idx]; taken {
				return fmt.Errorf("%w: device %d held by %s", ErrDeviceTaken, idx, holder)
			}
		}
		if l.Free() < len(c.DeviceIDs) {
			return l.shortfall(len(c.DeviceIDs))
		}
	default:
		if l.Free() < c.Count {
			return l.shortfall(c.Count)
		}
	}
	return nil
}

func (l Ledger) shortfall(need int) error {
	return fmt.Errorf("%w: needs %d, %d free of %d", ErrInsufficient, need, l.Free(), l.Total())
}
