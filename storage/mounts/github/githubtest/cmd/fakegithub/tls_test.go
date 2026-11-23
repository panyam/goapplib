package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// selfSigned writes a certificate for 127.0.0.1 and its key as PEM files in dir.
func selfSigned(t *testing.T, dir string) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fakegithub"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	pool = x509.NewCertPool()
	pool.AddCert(c)
	return certFile, keyFile, pool
}

func TestRunServesHTTPSWithACertificate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rev-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rev-b", "board.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(seed, []byte(`{"tokens": ["tok"],
	  "repos": [{"repo": "hw/baseboard", "private": true, "tokens": ["tok"], "commits": [{"tag": "rev-b", "dir": "rev-b"}]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, key, pool := selfSigned(t, dir)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"--seed", seed, "--listen", addr, "--tls-cert", cert, "--tls-key", key},
			io.Discard, func(c string) { ready <- c })
	}()
	var ctl string
	select {
	case ctl = <-ready:
	case err := <-done:
		t.Fatalf("run ended before it was ready: %v", err)
	}

	// The control address stays plain HTTP, and names the https API.
	resp, err := http.Get(ctl + "/")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		API string `json:"api"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if want := "https://" + addr + "/api/v3"; info.API != want {
		t.Errorf("GET / api = %q, want %q", info.API, want)
	}

	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/api/v3/repos/hw/baseboard/contents/board.txt", addr), nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err = c.Do(req)
	if err != nil {
		t.Fatalf("board.txt over HTTPS: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"board.txt"`) {
		t.Errorf("board.txt over HTTPS = %d %s, want 200", resp.StatusCode, b)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("run after cancel = %v, want nil", err)
	}
}

func TestRunWantsBothTLSFiles(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(seed, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, key, _ := selfSigned(t, dir)
	// A run that starts serving returns nil at the deadline, so the check fails instead of hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, args := range [][]string{{"--tls-cert", cert}, {"--tls-key", key}} {
		err := run(ctx, append([]string{"--seed", seed}, args...), io.Discard, nil)
		if err == nil || !strings.Contains(err.Error(), "--tls-cert and --tls-key") {
			t.Errorf("run %v = %v, want an error asking for both", args, err)
		}
	}
	if err := run(ctx, []string{"--seed", seed, "--tls-cert", key, "--tls-key", key}, io.Discard, nil); err == nil {
		t.Error("run with a key as the certificate = nil, want an error")
	}
}
