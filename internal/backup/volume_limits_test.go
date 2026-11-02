package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSanitize_OversizedNameRejectedFast(t *testing.T) {
	cases := map[string][]tarEntry{
		"long name":   {{name: strings.Repeat("a", 5000), body: "x"}},
		"deep name":   {{name: strings.Repeat("d/", 200) + "f", body: "x"}},
		"long target": {{name: "l", typ: tar.TypeSymlink, link: strings.Repeat("t", 5000)}},
	}
	for label, entries := range cases {
		_, err := sanitizeArchive(bytes.NewReader(buildArchive(t, entries)), io.Discard, 1<<20)
		if !errors.Is(err, ErrUnsafeArchive) {
			t.Errorf("%s: err = %v, want ErrUnsafeArchive", label, err)
		} else if len(err.Error()) > 1024 {
			t.Errorf("%s: error repeats the whole name (%d bytes)", label, len(err.Error()))
		}
	}
}

func TestSanitize_NameAndPaxBytesCountAgainstCap(t *testing.T) {
	name := strings.Repeat("n", 3000)
	arc := buildArchive(t, []tarEntry{{name: name, body: ""}})
	// One empty entry costs a block plus its name; a cap below that must trip.
	if _, err := sanitizeArchive(bytes.NewReader(arc), io.Discard, tarBlock+int64(len(name))-1); !errors.Is(err, ErrTransferTooLarge) {
		t.Fatalf("err = %v, want ErrTransferTooLarge", err)
	}
	st, err := sanitizeArchive(bytes.NewReader(arc), io.Discard, tarBlock+4*int64(len(name)))
	if err != nil || st.Bytes < tarBlock+int64(len(name)) {
		t.Fatalf("st=%+v err=%v", st, err)
	}
}

func TestSanitize_DropsNonBasicPaxRecords(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	h := &tar.Header{Name: "f", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1, Format: tar.FormatPAX,
		PAXRecords: map[string]string{"SCHILY.xattr.security.capability": "cap"}}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()
	var out bytes.Buffer
	if _, err := sanitizeArchive(&buf, &out, 1<<20); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("security.capability")) {
		t.Fatal("vendor pax record survived sanitizing")
	}
}

func TestSanitize_CancelledContextStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	arc := buildArchive(t, []tarEntry{{name: "a", body: "x"}})
	if _, err := sanitizeArchiveCtx(ctx, bytes.NewReader(arc), io.Discard, 1<<20, EntryLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestCheckNoImport(t *testing.T) {
	if err := CheckNoImport("web"); err != nil {
		t.Fatal(err)
	}
	done := MarkImport("web")
	if err := CheckNoImport("web"); !errors.Is(err, ErrImportInProgress) {
		t.Fatalf("err = %v", err)
	}
	done()
	if err := CheckNoImport("web"); err != nil {
		t.Fatal(err)
	}
}
