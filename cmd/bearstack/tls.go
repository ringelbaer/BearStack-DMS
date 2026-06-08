// Datei kapselt die TLS-Konfiguration und Zertifikatslogik fuer den Bearstack-Serverstart.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

	"bearstack/internal/config"
)

func tlsCertificateFiles(cfg config.Config) (string, string, error) {
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		return cfg.TLS.CertFile, cfg.TLS.KeyFile, nil
	}
	if !cfg.TLS.AutoCert {
		return "", "", fmt.Errorf("tls cert_file and key_file are required when auto_cert is disabled")
	}
	certFile := filepath.Join(cfg.DataDir, "tls", "bearstack-local.crt")
	keyFile := filepath.Join(cfg.DataDir, "tls", "bearstack-local.key")
	if fileExists(certFile) && fileExists(keyFile) {
		return certFile, keyFile, nil
	}
	if err := generateLocalTLSCertificate(certFile, keyFile, cfg.Addr); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}

func generateLocalTLSCertificate(certFile, keyFile, addr string) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0o700); err != nil {
		return fmt.Errorf("create tls directory: %w", err)
	}
	if keyDir := filepath.Dir(keyFile); keyDir != filepath.Dir(certFile) {
		if err := os.MkdirAll(keyDir, 0o700); err != nil {
			return fmt.Errorf("create tls key directory: %w", err)
		}
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate tls private key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate tls serial number: %w", err)
	}

	cert := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "BearStack local",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(397 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              localTLSDNSNames(addr),
		IPAddresses:           localTLSIPAddresses(addr),
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create tls certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("marshal tls private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return fmt.Errorf("write tls certificate: %w", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write tls private key: %w", err)
	}
	return nil
}

func localTLSDNSNames(addr string) []string {
	names := []string{"localhost"}
	if host := listenHost(addr); host != "" && net.ParseIP(host) == nil && host != "0.0.0.0" && host != "::" {
		names = append(names, host)
	}
	if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
		names = append(names, strings.TrimSpace(hostname))
	}
	return uniqueStrings(names)
}

func localTLSIPAddresses(addr string) []net.IP {
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if ip := net.ParseIP(listenHost(addr)); ip != nil && !ip.IsUnspecified() {
		ips = append(ips, ip)
	}
	return uniqueIPs(ips)
}

func listenHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return strings.Trim(host, "[]")
	}
	if strings.Count(addr, ":") == 0 {
		return strings.TrimSpace(addr)
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueIPs(values []net.IP) []net.IP {
	seen := make(map[string]struct{}, len(values))
	out := make([]net.IP, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		key := value.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
