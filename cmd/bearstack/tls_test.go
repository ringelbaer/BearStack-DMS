package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/config"
)

func TestTLSCertificateFilesUsesConfiguredFiles(t *testing.T) {
	certFile := filepath.Join(t.TempDir(), "server.crt")
	keyFile := filepath.Join(t.TempDir(), "server.key")
	gotCert, gotKey, err := tlsCertificateFiles(config.Config{
		TLS: config.TLSConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotCert != certFile || gotKey != keyFile {
		t.Fatalf("tls files = %q %q", gotCert, gotKey)
	}
}

func TestTLSCertificateFilesGeneratesLocalCertificate(t *testing.T) {
	dataDir := t.TempDir()
	certFile, keyFile, err := tlsCertificateFiles(config.Config{
		Addr:    "127.0.0.1:8443",
		DataDir: dataDir,
		TLS: config.TLSConfig{
			AutoCert: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("generated key pair is invalid: %v", err)
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("certificate PEM missing")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("localhost"); err != nil {
		t.Fatalf("localhost SAN missing: %v", err)
	}
	if err := cert.VerifyHostname("127.0.0.1"); err != nil {
		t.Fatalf("127.0.0.1 SAN missing: %v", err)
	}
	if filepath.Dir(certFile) != filepath.Join(dataDir, "tls") || filepath.Dir(keyFile) != filepath.Join(dataDir, "tls") {
		t.Fatalf("unexpected tls paths: %q %q", certFile, keyFile)
	}
}

func TestServerTLSConfigRequiresTLS12(t *testing.T) {
	cfg := serverTLSConfig()
	if cfg == nil {
		t.Fatal("TLS config is nil")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want %x", cfg.MinVersion, tls.VersionTLS12)
	}
}
