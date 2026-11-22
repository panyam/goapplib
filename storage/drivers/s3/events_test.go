package s3_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/drivers/s3"
	"github.com/panyam/goapplib/storage/mounts"
)

// s3Event is the body S3 sends to SQS for one put, trimmed from the example in AWS's docs. The
// key is form-encoded, as S3 sends it.
const s3Event = `{"Records":[{"eventVersion":"2.1","eventSource":"aws:s3","awsRegion":"us-east-1",
"eventTime":"2026-11-21T10:00:00.000Z","eventName":"ObjectCreated:Put",
"s3":{"s3SchemaVersion":"1.0","bucket":{"name":"designs","arn":"arn:aws:s3:::designs"},
"object":{"key":"boards/jetson/power+notes.md","size":42,"eTag":"0123456789abcdef","versionId":"v2"}}},
{"eventSource":"aws:s3","eventName":"ObjectRemoved:Delete",
"s3":{"bucket":{"name":"designs"},"object":{"key":"boards/jetson/old.kicad_sch"}}},
{"eventSource":"aws:s3","eventName":"ObjectRestore:Completed",
"s3":{"bucket":{"name":"designs"},"object":{"key":"boards/archive.zip"}}}]}`

func sources(cs []mounts.Change) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Source)
	}
	return out
}

func TestParseEvent(t *testing.T) {
	want := []string{"s3://designs/boards/jetson/power notes.md", "s3://designs/boards/jetson/old.kicad_sch"}
	got, err := s3.ParseEvent([]byte(s3Event))
	if err != nil || !slices.Equal(sources(got), want) {
		t.Fatalf("ParseEvent = %v, %v; want %v", sources(got), err, want)
	}
	wrapped, _ := json.Marshal(map[string]string{"Type": "Notification", "Message": s3Event})
	if got, err := s3.ParseEvent(wrapped); err != nil || !slices.Equal(sources(got), want) {
		t.Errorf("ParseEvent of an SNS envelope = %v, %v; want %v", sources(got), err, want)
	}
	if got, err := s3.ParseEvent([]byte(`{"Service":"Amazon S3","Event":"s3:TestEvent","Bucket":"designs"}`)); err != nil || len(got) != 0 {
		t.Errorf("ParseEvent of S3's test event = %v, %v; want nothing", got, err)
	}
	if _, err := s3.ParseEvent([]byte("not json")); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("ParseEvent of junk = %v, want ErrInvalidArgument", err)
	}
}

// snsEnv is a fake SNS signing host: a TLS server serving one certificate, and a verifier that
// trusts its host.
type snsEnv struct {
	key     *rsa.PrivateKey
	certURL string
	fetches int
	v       *s3.SNSVerifier
}

func newSNSEnv(t *testing.T) *snsEnv {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sns.amazonaws.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	e := &snsEnv{key: key}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SimpleNotificationService-test.pem":
			e.fetches++
			pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: der})
		case "/confirm":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	e.certURL = srv.URL + "/SimpleNotificationService-test.pem"
	host := strings.Split(strings.TrimPrefix(srv.URL, "https://"), ":")[0]
	e.v = &s3.SNSVerifier{Client: srv.Client(), AllowHost: func(h string) bool { return h == host }}
	return e
}

// message builds an SNS delivery signed with e's key, using the field order SNS signs in.
func (e *snsEnv) message(t *testing.T, typ, version string, fields map[string]string) map[string]string {
	t.Helper()
	m := map[string]string{"Type": typ, "MessageId": "m-1", "TopicArn": "arn:aws:sns:us-east-1:123:designs",
		"Timestamp": "2026-11-21T10:00:01.000Z", "SignatureVersion": version, "SigningCertURL": e.certURL}
	for k, v := range fields {
		m[k] = v
	}
	keys := []string{"Message", "MessageId", "Subject", "Timestamp", "TopicArn", "Type"}
	if typ != "Notification" {
		keys = []string{"Message", "MessageId", "SubscribeURL", "Timestamp", "Token", "TopicArn", "Type"}
	}
	var b strings.Builder
	for _, k := range keys {
		if v, ok := m[k]; ok {
			b.WriteString(k + "\n" + v + "\n")
		}
	}
	h, sum := crypto.SHA256, sha256.Sum256([]byte(b.String()))
	digest := sum[:]
	if version == "1" {
		s := sha1.Sum([]byte(b.String()))
		h, digest = crypto.SHA1, s[:]
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, e.key, h, digest)
	if err != nil {
		t.Fatal(err)
	}
	m["Signature"] = base64.StdEncoding.EncodeToString(sig)
	return m
}

func post(m map[string]string) *http.Request {
	b, _ := json.Marshal(m)
	return httptest.NewRequest(http.MethodPost, "/hooks/s3", strings.NewReader(string(b)))
}

func TestParseSNSNotification(t *testing.T) {
	e := newSNSEnv(t)
	for _, version := range []string{"1", "2"} {
		m, err := e.v.ParseSNS(post(e.message(t, "Notification", version, map[string]string{"Message": s3Event, "Subject": "Amazon S3 Notification"})))
		if err != nil {
			t.Fatalf("SignatureVersion %s: ParseSNS = %v", version, err)
		}
		if got := sources(m.Changes); len(got) != 2 || got[0] != "s3://designs/boards/jetson/power notes.md" {
			t.Errorf("SignatureVersion %s: Changes = %v", version, got)
		}
	}
	if e.fetches != 1 {
		t.Errorf("the certificate was fetched %d times, want once", e.fetches)
	}
}

func TestParseSNSRefusesForgeries(t *testing.T) {
	e := newSNSEnv(t)
	good := e.message(t, "Notification", "2", map[string]string{"Message": s3Event})

	tampered := maps(good)
	tampered["Message"] = strings.Replace(s3Event, "designs", "secrets", -1)
	badVersion := e.message(t, "Notification", "3", map[string]string{"Message": s3Event})
	elsewhere := maps(good)
	elsewhere["SigningCertURL"] = "https://evil.example.com/SimpleNotificationService-test.pem"
	plainHTTP := maps(good)
	plainHTTP["SigningCertURL"] = strings.Replace(e.certURL, "https://", "http://", 1)

	for name, m := range map[string]map[string]string{"tampered message": tampered, "SignatureVersion 3": badVersion,
		"certificate on another host": elsewhere, "certificate over http": plainHTTP} {
		before := e.fetches
		if got, err := e.v.ParseSNS(post(m)); !errors.Is(err, s3.ErrBadSignature) {
			t.Errorf("%s: ParseSNS = %+v, %v; want ErrBadSignature", name, got, err)
		}
		if name == "certificate on another host" && e.fetches != before {
			t.Errorf("%s: the verifier fetched a certificate", name)
		}
	}
	if _, err := (&s3.SNSVerifier{}).ParseSNS(post(elsewhere)); !errors.Is(err, s3.ErrBadSignature) {
		t.Errorf("the default host check let %q through: %v", elsewhere["SigningCertURL"], err)
	}
}

func TestParseSNSSubscriptionConfirmation(t *testing.T) {
	e := newSNSEnv(t)
	confirm := strings.Replace(e.certURL, "SimpleNotificationService-test.pem", "confirm", 1)
	m, err := e.v.ParseSNS(post(e.message(t, "SubscriptionConfirmation", "1",
		map[string]string{"Message": "You have chosen to subscribe", "SubscribeURL": confirm, "Token": "tok"})))
	if !errors.Is(err, s3.ErrSubscriptionConfirmation) || m == nil || m.SubscribeURL != confirm {
		t.Fatalf("ParseSNS = %+v, %v; want the message and ErrSubscriptionConfirmation", m, err)
	}
	if err := e.v.ConfirmSubscription(t.Context(), m); err != nil {
		t.Errorf("ConfirmSubscription = %v", err)
	}
	m.SubscribeURL = "https://169.254.169.254/latest/meta-data/"
	if err := e.v.ConfirmSubscription(t.Context(), m); !errors.Is(err, s3.ErrBadSignature) {
		t.Errorf("ConfirmSubscription to a non-SNS host = %v, want ErrBadSignature", err)
	}
}

func TestDefaultSNSHost(t *testing.T) {
	for host, want := range map[string]bool{"sns.us-east-1.amazonaws.com": true, "sns.cn-north-1.amazonaws.com.cn": true,
		"sns.us-east-1.amazonaws.com.evil.com": false, "evilsns.us-east-1.amazonaws.com": false, "s3.amazonaws.com": false} {
		if got := s3.DefaultSNSHost(host); got != want {
			t.Errorf("DefaultSNSHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func maps(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
