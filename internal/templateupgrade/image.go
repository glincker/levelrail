package templateupgrade

import (
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Image change kinds.
const (
	KindPatch     = "patch"
	KindMinor     = "minor"
	KindMajor     = "major"
	KindDowngrade = "downgrade"
	KindChanged   = "changed"
	KindNew       = "new"
	KindRemoved   = "removed"
)

// ImageChange is one service's image move. Preserved means the installed
// tag differs from the baseline, so the user pinned it and the executor
// leaves it alone.
type ImageChange struct {
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Kind      string `json:"kind"`
	Preserved bool   `json:"preserved,omitempty"`
}

func planImage(inst, tpl compose.Service, base *compose.Service, sig *signals) *ImageChange {
	if inst.Image == tpl.Image {
		return nil
	}
	ch := &ImageChange{From: inst.Image, To: tpl.Image, Kind: classifyImage(inst.Image, tpl.Image)}
	if base != nil && base.Image != inst.Image {
		ch.Preserved = true
		return ch
	}
	switch ch.Kind {
	case KindMajor:
		sig.major = true
	case KindMinor, KindPatch:
	default:
		sig.unclassified = true
	}
	return ch
}

type semver struct {
	nums   [3]int
	suffix string
}

func splitImage(image string) (repo, tag string) {
	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon > slash && !strings.Contains(image, "@") {
		return image[:colon], image[colon+1:]
	}
	return image, ""
}

func parseSemver(tag string) (semver, bool) {
	t := strings.TrimPrefix(tag, "v")
	end := 0
	for end < len(t) && (t[end] == '.' || (t[end] >= '0' && t[end] <= '9')) {
		end++
	}
	if end == 0 {
		return semver{}, false
	}
	parts := strings.Split(strings.TrimRight(t[:end], "."), ".")
	if len(parts) > 3 {
		return semver{}, false
	}
	var v semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, false
		}
		v.nums[i] = n
	}
	v.suffix = t[end:]
	return v, true
}

// classifyImage grades a tag move; anything but a same-repo, same-suffix
// semver step is a plain "changed".
func classifyImage(from, to string) string {
	fr, ft := splitImage(from)
	tr, tt := splitImage(to)
	if fr != tr {
		return KindChanged
	}
	a, okA := parseSemver(ft)
	b, okB := parseSemver(tt)
	if !okA || !okB || a.suffix != b.suffix {
		return KindChanged
	}
	for i, kind := range [3]string{KindMajor, KindMinor, KindPatch} {
		if a.nums[i] == b.nums[i] {
			continue
		}
		if b.nums[i] < a.nums[i] {
			return KindDowngrade
		}
		return kind
	}
	return KindChanged
}
