package proxyroutes

import (
	"bytes"
	"sort"
)

// File is one desired managed file.
type File struct {
	Domain  string
	Name    string
	Content []byte
}

// Outcome is what happened to one desired file.
type Outcome struct {
	Domain string
	Name   string
	// Changed is true when the file was created or its content replaced.
	Changed bool
	Err     error
}

// Removal is one managed file that is no longer wanted.
type Removal struct {
	Domain string
	Name   string
	Err    error
}

// ApplyResult is the outcome of one Apply pass.
type ApplyResult struct {
	Files   []Outcome
	Removed []Removal
	Foreign []string
}

// Apply converges the directory on want: writes missing or different files,
// leaves identical ones alone, and removes managed files no longer wanted.
// Safe to repeat after an interruption; files it did not write are never
// touched.
func Apply(d *Dir, want []File) (ApplyResult, error) {
	have, foreign, err := d.Managed()
	if err != nil {
		return ApplyResult{}, err
	}
	res := ApplyResult{Foreign: foreign}
	wanted := make(map[string]bool, len(want))
	for _, f := range want {
		wanted[f.Name] = true
		out := Outcome{Domain: f.Domain, Name: f.Name}
		if cur, ok := have[f.Name]; !ok || !bytes.Equal(cur, f.Content) {
			out.Err = d.Write(f.Name, f.Content)
			out.Changed = out.Err == nil
		}
		res.Files = append(res.Files, out)
	}
	names := make([]string, 0, len(have))
	for name := range have {
		if !wanted[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		res.Removed = append(res.Removed, Removal{Domain: d.DomainOf(name), Name: name, Err: d.Remove(name)})
	}
	return res, nil
}
