package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	caValidityYears     = 10
	serverValidityYears = 1
	tlsCACertFile       = "ca.crt"
	tlsCAKeyFile        = "ca.key"
	tlsServerCertFile   = "server.crt"
	tlsServerKeyFile    = "server.key"
)

// TLSBundle holds the generated CA and server certificate materials.
type TLSBundle struct {
	CACertPEM     []byte
	CAKeyPEM      []byte
	ServerCertPEM []byte
	ServerKeyPEM  []byte
	CAFingerprint string // SHA-256 fingerprint in colon-separated hex
}

// GenerateTLSBundle creates a self-signed CA and a server certificate signed by it.
// The server cert includes SANs for localhost, 127.0.0.1, and any additional IPs provided
// (typically the engine's WireGuard control tunnel IP 10.200.0.1 and host IP).
func GenerateTLSBundle(extraIPs ...net.IP) (*TLSBundle, error) {
	// Generate CA key pair
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	caSerial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: caSerial,
		Subject: pkix.Name{
			Organization: []string{"Banyan"},
			CommonName:   "Banyan CA",
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(caValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Generate server key pair
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serverSerial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	// Build SANs: always include localhost + loopback
	ips := []net.IP{net.IPv4(127, 0, 0, 1)}
	ips = append(ips, extraIPs...)
	dnsNames := []string{"localhost"}

	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject: pkix.Name{
			Organization: []string{"Banyan"},
			CommonName:   "Banyan Engine",
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(serverValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           ips,
		DNSNames:              dnsNames,
		BasicConstraintsValid: true,
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	// Encode to PEM
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CA key: %w", err)
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal server key: %w", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	return &TLSBundle{
		CACertPEM:     caCertPEM,
		CAKeyPEM:      caKeyPEM,
		ServerCertPEM: serverCertPEM,
		ServerKeyPEM:  serverKeyPEM,
		CAFingerprint: Fingerprint(caCertDER),
	}, nil
}

// Fingerprint returns the SHA-256 fingerprint of a DER-encoded certificate
// in colon-separated uppercase hex (e.g., "AB:CD:12:34:...").
func Fingerprint(certDER []byte) string {
	hash := sha256.Sum256(certDER)
	parts := make([]string, len(hash))
	for i, b := range hash {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// FingerprintFromPEM returns the SHA-256 fingerprint of a PEM-encoded certificate.
func FingerprintFromPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM certificate")
	}
	return Fingerprint(block.Bytes), nil
}

// SaveTLSBundle writes the TLS bundle to files in the given directory.
// Private key files are written with 0600 permissions.
func SaveTLSBundle(dir string, bundle *TLSBundle) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create TLS directory: %w", err)
	}

	files := []struct {
		name string
		data []byte
		perm os.FileMode
	}{
		{tlsCACertFile, bundle.CACertPEM, 0o644},
		{tlsCAKeyFile, bundle.CAKeyPEM, 0o600},
		{tlsServerCertFile, bundle.ServerCertPEM, 0o644},
		{tlsServerKeyFile, bundle.ServerKeyPEM, 0o600},
	}

	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, f.data, f.perm); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.name, err)
		}
	}
	return nil
}

// LoadServerTLSConfig loads the server cert+key and returns a tls.Config
// suitable for a gRPC or HTTP server.
func LoadServerTLSConfig(dir string) (*tls.Config, error) {
	certFile := filepath.Join(dir, tlsServerCertFile)
	keyFile := filepath.Join(dir, tlsServerKeyFile)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server TLS keypair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// LoadClientTLSConfig loads the CA cert and returns a tls.Config
// suitable for a gRPC or HTTP client that trusts the Banyan CA.
func LoadClientTLSConfig(caCertPath string) (*tls.Config, error) {
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	return &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}, nil
}

// TLSBundleExists checks whether TLS files already exist in the directory.
func TLSBundleExists(dir string) bool {
	for _, name := range []string{tlsCACertFile, tlsServerCertFile, tlsServerKeyFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

func randomSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	return serial, nil
}
