package gateway

import "testing"

func TestSelfSignedCertGeneration(t *testing.T) {
	cfg, err := setupTLS(TLSConfig{Enabled: true, Domain: "localhost", CertDir: t.TempDir(), AutoCert: false})
	if err != nil {
		t.Fatalf("setupTLS error: %v", err)
	}
	if cfg == nil || len(cfg.Certificates) == 0 {
		t.Fatal("expected self-signed certificate in tls.Config")
	}
}

func TestTLSConfigValidation(t *testing.T) {
	if _, err := setupTLS(TLSConfig{Enabled: true, AutoCert: true, Domain: "", CertDir: t.TempDir()}); err == nil {
		t.Fatal("expected validation error for missing domain with autocert")
	}
}
