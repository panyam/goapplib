package connections_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/mounts"
)

func TestAWSCredentialsResolveTheSecret(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOAPPLIB_TEST_AWS_SECRET", "wJalrXUtnFEMI")
	c := connections.Connection{ID: "s3", Kind: connections.KindAWS, Scope: connections.ScopeServer,
		AccessKeyID: "AKIAEXAMPLE", KeyRef: "env:GOAPPLIB_TEST_AWS_SECRET", RoleARN: "arn:aws:iam::1:role/r", Region: "eu-west-1"}
	if err := f.store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Credentials(context.Background(), "", "s3")
	if err != nil {
		t.Fatal(err)
	}
	want := mounts.AWSCredentials{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "wJalrXUtnFEMI", RoleARN: "arn:aws:iam::1:role/r", Region: "eu-west-1"}
	if got != want {
		t.Fatalf("Credentials = %#v, want %#v", got, want)
	}
}

func TestAWSWithoutAKeyUsesTheDefaultChain(t *testing.T) {
	f := newFixture(t)
	c := connections.Connection{ID: "s3", Kind: connections.KindAWS, Scope: connections.ScopeServer}
	if err := f.store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Credentials(context.Background(), "", "s3")
	if err != nil {
		t.Fatal(err)
	}
	if got != (mounts.AWSCredentials{DefaultChain: true}) {
		t.Fatalf("Credentials = %#v, want the default chain", got)
	}
}

// fakeGoogle writes a service account key whose token_uri is a fake token endpoint, and returns
// the key's path. The endpoint hands out "ya29.fake" for any signed assertion.
func fakeGoogle(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("assertion") == "" {
			http.Error(w, "no assertion", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ya29.fake","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: must(x509.MarshalPKCS8PrivateKey(k))})
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "p", "private_key_id": "k1", "private_key": string(keyPEM),
		"client_email": "reader@p.iam.gserviceaccount.com", "client_id": "1", "token_uri": srv.URL,
	})
	p := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestGCPServiceAccountKeyGivesABearerToken(t *testing.T) {
	f := newFixture(t)
	c := connections.Connection{ID: "gcs", Kind: connections.KindGCP, Scope: connections.ScopeServer, KeyRef: "file:" + fakeGoogle(t)}
	if err := f.store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if tok := token(t)(f.svc.Credentials(context.Background(), "", "gcs")); tok.AccessToken != "ya29.fake" {
		t.Fatalf("token = %q, want ya29.fake", tok.AccessToken)
	}
}

func TestGCPWithoutAKeyUsesApplicationDefaultCredentials(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", fakeGoogle(t))
	c := connections.Connection{ID: "gcs", Kind: connections.KindGCP, Scope: connections.ScopeServer}
	if err := f.store.PutConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if tok := token(t)(f.svc.Credentials(context.Background(), "", "gcs")); tok.AccessToken != "ya29.fake" {
		t.Fatalf("token = %q, want ya29.fake", tok.AccessToken)
	}
}
