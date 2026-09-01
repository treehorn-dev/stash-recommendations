package auth

import "testing"

func TestAPIKeyHashVerifiesOnlyItsPlaintextValue(t *testing.T) {
	hash, err := HashAPIKey("issued-key")
	if err != nil {
		t.Fatalf("HashAPIKey() error = %v", err)
	}

	if !VerifyAPIKey(hash, "issued-key") {
		t.Fatal("VerifyAPIKey() accepted neither its plaintext key nor hash")
	}
	if VerifyAPIKey(hash, "other-key") {
		t.Fatal("VerifyAPIKey() accepted an arbitrary key")
	}
}
