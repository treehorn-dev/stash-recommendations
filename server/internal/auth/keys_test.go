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

func TestNewAPIKeyCarriesSeparateIdentifierAndSecret(t *testing.T) {
	plaintext, err := NewAPIKey()
	if err != nil {
		t.Fatalf("NewAPIKey() error = %v", err)
	}

	identifier, secret, ok := ParseAPIKey(plaintext)
	if !ok {
		t.Fatalf("ParseAPIKey(%q) = invalid", plaintext)
	}
	if identifier == "" || secret == "" {
		t.Fatalf("ParseAPIKey() = identifier %q, secret %q", identifier, secret)
	}
	if identifier == secret {
		t.Fatal("identifier must not be the API key secret")
	}
}
