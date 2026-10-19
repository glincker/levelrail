package build

import "testing"

func TestAttestSniffer_TruncatedStreamIsMarkedIncomplete(t *testing.T) {
	s := newAttestSniffer(0)
	if _, err := s.Write([]byte("this is not a tar archive, just garbage bytes that tar cannot parse as a header block")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got := s.Finish()
	if !got.Incomplete || got.ScanError == "" {
		t.Errorf("Finish() = %+v, want Incomplete with a scan error", got)
	}
	if len(got.SBOM) != 0 || len(got.Provenance) != 0 {
		t.Errorf("Finish() = %+v, want no attestations from a broken stream", got)
	}
}
