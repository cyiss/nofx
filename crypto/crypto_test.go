package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestRejectPublicDataKeyPlaceholder(t *testing.T) {
	for _, key := range []string{"", "your-base64-encoded-32-byte-key"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(EnvDataEncryptionKey, key)
			if _, err := loadDataKeyFromEnv(); err == nil {
				t.Fatal("accepted missing or publicly documented data key")
			}
		})
	}
}

func TestExistingDataKeysRemainCompatible(t *testing.T) {
	raw := []byte{1, 12, 53, 41, 150, 234, 90, 43, 8, 12, 14, 65, 73, 54, 43, 39, 20, 218, 237, 43, 1, 253, 54, 67, 86, 189, 3, 32, 42, 213, 63, 87}
	legacy := "existing-private-passphrase-with-non-base64-characters!"
	legacySum := sha256.Sum256([]byte(legacy))
	for _, tc := range []struct {
		name, encoded string
		key           []byte
	}{
		{"base64", base64.StdEncoding.EncodeToString(raw), raw},
		{"raw-base64", base64.RawStdEncoding.EncodeToString(raw), raw},
		// Keep historical decoder order for hex input, including its base64 interpretation.
		{"legacy-passphrase", legacy, legacySum[:]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvDataEncryptionKey, tc.encoded)
			key, err := loadDataKeyFromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(key, tc.key) {
				t.Fatal("existing key derivation changed")
			}
			old := &CryptoService{dataKey: tc.key}
			ciphertext, err := old.EncryptForStorage("private-wallet-credential")
			if err != nil {
				t.Fatal(err)
			}
			plaintext, err := (&CryptoService{dataKey: key}).DecryptFromStorage(ciphertext)
			if err != nil || plaintext != "private-wallet-credential" {
				t.Fatalf("existing ciphertext no longer decrypts: %v", err)
			}
		})
	}
	// Hex strings are already accepted by legacy versions. Preserve the exact
	// historical derived bytes instead of changing stored ciphertext semantics.
	hexValue := hex.EncodeToString(raw)
	want, err := hex.DecodeString("0d20abcd28243b8a6ce3c1dd08e2466d1e8dcfb19d264c646d9cb94fac5f7177")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDataEncryptionKey, hexValue)
	got, err := loadDataKeyFromEnv()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("legacy hex interpretation changed")
	}
}
