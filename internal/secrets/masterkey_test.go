package secrets

import "testing"

func TestGenerateMasterKey_ProducesUsableKey(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	if mk.String() == "" {
		t.Error("String() = empty, want a serialized identity")
	}
}

func TestGenerateMasterKey_ProducesDistinctKeys(t *testing.T) {
	a, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	b, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	if a.String() == b.String() {
		t.Error("two calls to GenerateMasterKey produced the same key, want distinct keys")
	}
}

func TestLoadMasterKey_RoundTrip(t *testing.T) {
	original, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}

	loaded, err := LoadMasterKey(original.String())
	if err != nil {
		t.Fatalf("LoadMasterKey() error = %v", err)
	}
	if loaded.String() != original.String() {
		t.Error("LoadMasterKey(original.String()) did not round-trip to the same key")
	}

	// The real test of "did this round-trip correctly": a DEK wrapped
	// under the original key must be unwrappable by the loaded one, not
	// just that the two String() values match textually.
	_, wrapped, err := original.GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}
	if _, err := loaded.UnwrapDEK(wrapped); err != nil {
		t.Errorf("loaded key could not unwrap a DEK wrapped by the original key: %v", err)
	}
}

func TestLoadMasterKey_InvalidInput(t *testing.T) {
	_, err := LoadMasterKey("not a valid age identity")
	if err == nil {
		t.Fatal("LoadMasterKey() error = nil, want an error for malformed input")
	}
}
