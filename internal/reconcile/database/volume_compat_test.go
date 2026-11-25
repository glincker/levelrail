package database

import (
	"context"
	"errors"
	"testing"
)

type fakeVolReader map[string]string

func (f fakeVolReader) ReadVolumeFile(_ context.Context, vol, rel string) ([]byte, error) {
	if s, ok := f[vol+"/"+rel]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("missing")
}

func TestVolumeCompatMajors(t *testing.T) {
	r := fakeVolReader{"v/PG_VERSION": "18\n", "m/mysql_upgrade_info": "8.0.36", "d/mariadb_upgrade_info": "10.11.6-MariaDB"}
	cases := []struct {
		engine, vol, ask string
		major            string
		ok               bool
	}{
		{"postgres", "v", "19.0", "18", false},
		{"postgres", "v", "18.7", "18", true},
		{"mysql", "m", "8.4", "8.0", false},
		{"mysql", "m", "8.0.40", "8.0", true},
		{"mariadb", "d", "10.11", "10.11", true},
		{"redis", "v", "7", "", true},
		{"postgres", "none", "19", "", true},
	}
	for _, c := range cases {
		maj := DetectVolumeMajor(context.Background(), r, c.engine, c.vol)
		if maj != c.major || VolumeCompatible(c.engine, maj, c.ask) != c.ok {
			t.Errorf("%s %s ask %s: major %q ok %v", c.engine, c.vol, c.ask, maj, VolumeCompatible(c.engine, maj, c.ask))
		}
	}
}
