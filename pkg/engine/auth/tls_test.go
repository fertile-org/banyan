package auth

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTLSBundle_HappyPath(t *testing.T) {
	bundle, err := GenerateTLSBundle()
	if err != nil {
		t.Fatalf("GenerateTLSBundle: %v", err)
	}

	if len(bundle.CACertPEM) == 0 {
		t.Error("CA cert PEM should not be empty")
	}
	if len(bundle.CAKeyPEM) == 0 {
		t.Error("CA key PEM should not be empty")
	}
	if len(bundle.ServerCertPEM) == 0 {
		t.Error("server cert PEM should not be empty")
	}
	if len(bundle.ServerKeyPEM) == 0 {
		t.Error("server key PEM should not be empty")
	}
	if bundle.CAFingerprint == "" {
		t.Error("CA fingerprint should not be empty")
	}
}

func TestGenerateTLSBundle_CACertIsValid(t *testing.T) {
	bundle, err := GenerateTLSBundle()
	if err != nil {
		t.Fatalf("GenerateTLSBundle: %v", err)
	}

	block, _ := pem.Decode(bundle.CACertPEM)
	if block == nil {
		t.Fatal("failed to decode CA cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA cert: %v", err)
	}

	if !cert.IsCA {
		t.Error("CA cert should have IsCA=true")
	}
	if cert.Subject.CommonName != "Banyan CA" {
		t.Errorf("CA CN = %q, want 'Banyan CA'", cert.Subject.CommonName)
	}
}

func TestGenerateTLSBundle_ServerCertSignedByCA(t *testing.T) {
	bundle, err := GenerateTLSBundle()
	if err != nil {
		t.Fatalf("GenerateTLSBundle: %v", err)
	}

	// Parse CA
	caBlock, _ := pem.Decode(bundle.CACertPEM)
	caCert, _ := x509.ParseCertificate(caBlock.Bytes)

	// Parse server cert
	serverBlock, _ := pem.Decode(bundle.ServerCertPEM)
	serverCert, err := x509.ParseCertificate(serverBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse server cert: %v", err)
	}

	// Verify server cert is signed by CA
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	_, err = serverCert.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		t.Fatalf("server cert verification failed: %v", err)
	}
}

func TestGenerateTLSBundle_ServerCertSANs(t *testing.T) {
	extraIP := net.ParseIP("10.200.0.1")
	bundle, err := GenerateTLSBundle(extraIP)
	if err != nil {
		t.Fatalf("GenerateTLSBundle: %v", err)
	}

	serverBlock, _ := pem.Decode(bundle.ServerCertPEM)
	serverCert, _ := x509.ParseCertificate(serverBlock.Bytes)

	// Check DNS SANs
	foundLocalhost := false
	for _, dns := range serverCert.DNSNames {
		if dns == "localhost" {
			foundLocalhost = true
		}
	}
	if !foundLocalhost {
		t.Error("server cert should include 'localhost' in DNS SANs")
	}

	// Check IP SANs
	foundLoopback := false
	foundExtra := false
	for _, ip := range serverCert.IPAddresses {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			foundLoopback = true
		}
		if ip.Equal(extraIP) {
			foundExtra = true
		}
	}
	if !foundLoopback {
		t.Error("server cert should include 127.0.0.1 in IP SANs")
	}
	if !foundExtra {
		t.Error("server cert should include extra IP 10.200.0.1 in IP SANs")
	}
}

func TestFingerprint_Format(t *testing.T) {
	bundle, _ := GenerateTLSBundle()

	fp := bundle.CAFingerprint
	parts := strings.Split(fp, ":")
	if len(parts) != 32 { // SHA-256 = 32 bytes
		t.Errorf("fingerprint should have 32 colon-separated parts, got %d", len(parts))
	}
	for _, p := range parts {
		if len(p) != 2 {
			t.Errorf("each fingerprint part should be 2 chars, got %q", p)
		}
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	bundle, _ := GenerateTLSBundle()

	fp1 := bundle.CAFingerprint
	fp2, err := FingerprintFromPEM(bundle.CACertPEM)
	if err != nil {
		t.Fatalf("FingerprintFromPEM: %v", err)
	}

	if fp1 != fp2 {
		t.Error("fingerprint should be deterministic for the same cert")
	}
}

func TestFingerprintFromPEM_Invalid(t *testing.T) {
	_, err := FingerprintFromPEM([]byte("not a pem"))
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestSaveAndLoadTLSBundle(t *testing.T) {
	dir := t.TempDir()
	bundle, _ := GenerateTLSBundle(net.ParseIP("10.200.0.1"))

	if err := SaveTLSBundle(dir, bundle); err != nil {
		t.Fatalf("SaveTLSBundle: %v", err)
	}

	// Verify files exist with correct permissions
	keyFiles := []string{tlsCAKeyFile, tlsServerKeyFile}
	for _, name := range keyFiles {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("key file %s not found: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s permissions = %o, want 0600", name, info.Mode().Perm())
		}
	}

	certFiles := []string{tlsCACertFile, tlsServerCertFile}
	for _, name := range certFiles {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("cert file %s not found: %v", name, err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("%s permissions = %o, want 0644", name, info.Mode().Perm())
		}
	}

	// Load server TLS config
	serverCfg, err := LoadServerTLSConfig(dir)
	if err != nil {
		t.Fatalf("LoadServerTLSConfig: %v", err)
	}
	if len(serverCfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(serverCfg.Certificates))
	}
	if serverCfg.MinVersion != tls.VersionTLS12 {
		t.Error("MinVersion should be TLS 1.2")
	}

	// Load client TLS config
	clientCfg, err := LoadClientTLSConfig(filepath.Join(dir, tlsCACertFile))
	if err != nil {
		t.Fatalf("LoadClientTLSConfig: %v", err)
	}
	if clientCfg.RootCAs == nil {
		t.Error("RootCAs should not be nil")
	}
}

func TestLoadServerTLSConfig_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadServerTLSConfig(dir)
	if err == nil {
		t.Error("expected error when cert files are missing")
	}
}

func TestLoadClientTLSConfig_InvalidCA(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.crt")
	_ = os.WriteFile(badFile, []byte("not a cert"), 0o644)

	_, err := LoadClientTLSConfig(badFile)
	if err == nil {
		t.Error("expected error for invalid CA cert")
	}
}

func TestLoadClientTLSConfig_MissingFile(t *testing.T) {
	_, err := LoadClientTLSConfig("/nonexistent/ca.crt")
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

func TestTLSBundleExists(t *testing.T) {
	dir := t.TempDir()

	if TLSBundleExists(dir) {
		t.Error("should return false for empty directory")
	}

	bundle, _ := GenerateTLSBundle()
	_ = SaveTLSBundle(dir, bundle)

	if !TLSBundleExists(dir) {
		t.Error("should return true after saving bundle")
	}
}

func TestTLSBundleExists_PartialFiles(t *testing.T) {
	dir := t.TempDir()
	// Only write CA cert, not server cert/key
	_ = os.WriteFile(filepath.Join(dir, tlsCACertFile), []byte("cert"), 0o644)

	if TLSBundleExists(dir) {
		t.Error("should return false when files are incomplete")
	}
}
