package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestReadSigningKeyRequiresSecretFile(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	path := filepath.Join(t.TempDir(), "signing-key")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(seed)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SIGNING_KEY_FILE", path)
	key, err := readSigningKeyFileEnv("TEST_SIGNING_KEY_FILE")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != ed25519.PrivateKeySize {
		t.Fatalf("private key length = %d", len(key))
	}
}

func TestBoolEnvRejectsInvalidValue(t *testing.T) {
	t.Setenv("TEST_BOOLEAN", "definitely")
	if _, err := boolEnv("TEST_BOOLEAN", true); err == nil {
		t.Fatal("invalid boolean must reject startup")
	}
}

func TestRequiredEnvOrFileReadsSecretFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(path, []byte("postgres://civilization@example/civilization\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_DATABASE_URL_FILE", path)
	value, err := requiredEnvOrFile("TEST_DATABASE_URL")
	if err != nil {
		t.Fatal(err)
	}
	if value != "postgres://civilization@example/civilization" {
		t.Fatalf("value = %q", value)
	}
}

func TestRequiredEnvOrFileRejectsEmptySecretFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_DATABASE_URL_FILE", path)
	if _, err := requiredEnvOrFile("TEST_DATABASE_URL"); err == nil {
		t.Fatal("empty secret file must reject startup")
	}
}
