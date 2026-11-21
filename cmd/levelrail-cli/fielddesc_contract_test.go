package main

import (
	"flag"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fielddesc"
)

// Every descriptor flag must register on a flag set. Locale keys, docs rows and the flag or
// form only rule for each resource are held by internal/fielddesc's own completeness test.
func TestFieldDescriptorFlagsRegister(t *testing.T) {
	for _, name := range fielddesc.Names() {
		res, _ := fielddesc.Get(name)
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		registerDescriptorFlags(fs, name)
		for _, f := range res.Fields {
			if f.Flag != "" && fs.Lookup(f.Flag) == nil {
				t.Errorf("%s.%s: flag --%s not registered", name, f.ID, f.Flag)
			}
		}
	}
}
