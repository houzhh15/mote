// Package main provides certificate generation utility for SDP demo
package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

var (
	outDir     = flag.String("out", "./certs", "Output directory for certificates")
	commonName = flag.String("cn", "SDP Demo CA", "Common name for CA")
	days       = flag.Int("days", 365, "Validity days")
)

func main() {
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Generating SDP demo certificates...")

	// Generate CA
	caCert, caKey, err := generateCA()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate CA: %v\n", err)
		os.Exit(1)
	}
	if err := saveCert(caCert, "ca.pem"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save CA cert: %v\n", err)
		os.Exit(1)
	}
	if err := saveKey(caKey, "ca.key"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save CA key: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  CA certificate generated")

	// Generate Server certificate
	serverCert, serverKey, err := generateCert(caCert, caKey, "SDP Server", []string{"localhost", "127.0.0.1"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate server certificate: %v\n", err)
		os.Exit(1)
	}
	if err := saveCert(serverCert, "server.pem"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save server cert: %v\n", err)
		os.Exit(1)
	}
	if err := saveKey(serverKey, "server.key"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save server key: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Server certificate generated")

	// Generate Client certificate
	clientCert, clientKey, err := generateCert(caCert, caKey, "SDP Client", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate client certificate: %v\n", err)
		os.Exit(1)
	}
	if err := saveCert(clientCert, "client.pem"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save client cert: %v\n", err)
		os.Exit(1)
	}
	if err := saveKey(clientKey, "client.key"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save client key: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Client certificate generated")

	fmt.Printf("\nCertificates generated successfully in: %s\n", *outDir)
	fmt.Println("Files created:")
	fmt.Println("  ca.pem, ca.key     - CA certificate and key")
	fmt.Println("  server.pem, server.key - Server certificate and key")
	fmt.Println("  client.pem, client.key - Client certificate and key")
}

func generateCA() (*x509.Certificate, interface{}, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"SDP Demo"},
			CommonName:   *commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	return cert, key, err
}

func generateCert(caCert *x509.Certificate, caKey interface{}, name string, hosts []string) (*x509.Certificate, interface{}, error) {
	var key interface{}
	var err error

	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"SDP Demo"},
			CommonName:   name,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	if len(hosts) > 0 {
		for _, h := range hosts {
			if ip := net.ParseIP(h); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			}
		}
	}

	caPrivKey, ok := caKey.(crypto.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("invalid CA key type")
	}

	var pubKey crypto.PublicKey
	switch k := key.(type) {
	case *rsa.PrivateKey:
		pubKey = &k.PublicKey
	case *ecdsa.PrivateKey:
		pubKey = &k.PublicKey
	default:
		return nil, nil, fmt.Errorf("unsupported key type")
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, pubKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	return cert, key, err
}

func saveCert(cert *x509.Certificate, filename string) error {
	path := filepath.Join(*outDir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func saveKey(key interface{}, filename string) error {
	path := filepath.Join(*outDir, filename)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var b []byte
	switch k := key.(type) {
	case *rsa.PrivateKey:
		b = x509.MarshalPKCS1PrivateKey(k)
	case *ecdsa.PrivateKey:
		b, _ = x509.MarshalECPrivateKey(k)
	default:
		return fmt.Errorf("unsupported key type")
	}

	return pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: b})
}

// GetTLSConfig returns a TLS configuration for server
func GetTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
