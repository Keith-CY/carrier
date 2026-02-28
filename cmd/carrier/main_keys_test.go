package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeysGenerateCreatesEd25519Key(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out bytes.Buffer
	if err := runKeysGenerate(&out, "testkey"); err != nil {
		t.Fatalf("runKeysGenerate error: %v", err)
	}

	privatePath := filepath.Join(home, ".carrier", "keys", "testkey")
	pubPath := privatePath + ".pub"
	privateRaw, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("read private key: %v", err)
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("public key file not found: %v", err)
	}

	block, _ := pem.Decode(privateRaw)
	if block == nil {
		t.Fatal("failed to parse PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse PKCS8 private key: %v", err)
	}
	if _, ok := parsed.(ed25519.PrivateKey); !ok {
		t.Fatalf("private key type = %T, want ed25519.PrivateKey", parsed)
	}
}

func TestRunKeysListShowsFingerprint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := runKeysGenerate(bytes.NewBuffer(nil), "alpha"); err != nil {
		t.Fatalf("runKeysGenerate error: %v", err)
	}

	var out bytes.Buffer
	if err := runKeysList(&out); err != nil {
		t.Fatalf("runKeysList error: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "alpha") {
		t.Fatalf("expected key alias in output, got %q", text)
	}
	if !strings.Contains(text, "SHA256:") {
		t.Fatalf("expected fingerprint in output, got %q", text)
	}
}

func TestRunKeysDeleteRemovesKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := runKeysGenerate(bytes.NewBuffer(nil), "deleteme"); err != nil {
		t.Fatalf("runKeysGenerate error: %v", err)
	}
	var out bytes.Buffer
	if err := runKeysDelete(&out, "deleteme"); err != nil {
		t.Fatalf("runKeysDelete error: %v", err)
	}

	privatePath := filepath.Join(home, ".carrier", "keys", "deleteme")
	if _, err := os.Stat(privatePath); !os.IsNotExist(err) {
		t.Fatalf("expected private key deleted, stat err=%v", err)
	}
	if _, err := os.Stat(privatePath + ".pub"); !os.IsNotExist(err) {
		t.Fatalf("expected public key deleted, stat err=%v", err)
	}
}
