package s3

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- SNS SignatureVersion 1 is SHA1withRSA, and AWS still sends it by default
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/panyam/goapplib/storage/mounts"
)

// The errors ParseSNS returns. An app's handler maps them onto its answer with errors.Is.
var (
	// ErrBadSignature means the request isn't a message SNS signed: the body isn't an SNS
	// message, the signing certificate isn't on an SNS host, or the signature doesn't match.
	// Answer 401 or 403.
	ErrBadSignature = errors.New("s3: SNS message not signed by SNS")

	// ErrSubscriptionConfirmation means the message asks the app to confirm a new SNS
	// subscription, or tells it one was removed. The returned SNSMessage carries SubscribeURL;
	// call ConfirmSubscription to accept, or ignore it to refuse. Answer 200 either way.
	ErrSubscriptionConfirmation = errors.New("s3: SNS subscription confirmation")
)

// ParseEvent reads an S3 event notification, as S3 delivers it to SQS, or as SNS wraps it, and
// returns one Change per object created or removed. Each Change's Source names the object, such as
// "s3://designs/boards/jetson/README.md", so Registry.Following finds every mount whose prefix holds
// it. Its Ref and Version are empty, since a bucket mount has no ref and an object event doesn't
// say what the mount's listing hash becomes; an app re-resolves the mounts it finds. S3's test
// event and other event types return no changes and no error.
func ParseEvent(body []byte) ([]mounts.Change, error) {
	var env struct {
		Type    string `json:"Type"`
		Message string `json:"Message"`
	}
	if json.Unmarshal(body, &env) == nil && env.Type == "Notification" && env.Message != "" {
		body = []byte(env.Message)
	}
	var ev struct {
		Records []struct {
			EventSource string `json:"eventSource"`
			EventName   string `json:"eventName"`
			S3          struct {
				Bucket struct {
					Name string `json:"name"`
				} `json:"bucket"`
				Object struct {
					Key string `json:"key"`
				} `json:"object"`
			} `json:"s3"`
		} `json:"Records"`
		Event string `json:"Event"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, fmt.Errorf("%w: not an S3 event: %v", mounts.ErrInvalidArgument, err)
	}
	var out []mounts.Change
	for _, r := range ev.Records {
		if r.EventSource != "aws:s3" {
			continue
		}
		if !strings.HasPrefix(r.EventName, "ObjectCreated:") && !strings.HasPrefix(r.EventName, "ObjectRemoved:") {
			continue
		}
		// S3 sends the key form-encoded, so a space arrives as "+".
		key, err := url.QueryUnescape(r.S3.Object.Key)
		if err != nil || r.S3.Bucket.Name == "" || key == "" {
			return nil, fmt.Errorf("%w: S3 event names bucket %q and key %q", mounts.ErrInvalidArgument, r.S3.Bucket.Name, r.S3.Object.Key)
		}
		out = append(out, mounts.Change{Source: "s3://" + r.S3.Bucket.Name + "/" + key})
	}
	return out, nil
}

// SNSMessage is one message SNS delivered to an HTTPS subscription.
type SNSMessage struct {
	Type      string
	MessageID string
	TopicArn  string
	Subject   string
	Message   string
	Timestamp string

	// SubscribeURL and Token are set on a SubscriptionConfirmation or UnsubscribeConfirmation.
	SubscribeURL string
	Token        string

	// Changes are the S3 changes a Notification carries, from ParseEvent.
	Changes []mounts.Change

	signatureVersion string
	signature        string
	signingCertURL   string
}

// SNSVerifier checks SNS message signatures. Its zero value is ready to use and fetches signing
// certificates with http.DefaultClient. Share one per process, since it caches certificates by URL.
type SNSVerifier struct {
	// Client fetches signing certificates and confirms subscriptions. Nil means
	// http.DefaultClient.
	Client *http.Client

	// AllowHost reports whether a certificate or subscribe URL's host is SNS's. Nil means
	// DefaultSNSHost. A test points it at its own server.
	AllowHost func(host string) bool

	mu    sync.Mutex
	certs map[string]*x509.Certificate
}

var snsHost = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

// DefaultSNSHost reports whether host is a regional SNS endpoint, such as
// "sns.us-east-1.amazonaws.com". SNS serves its signing certificates only from these hosts, so a
// message naming a certificate anywhere else is forged.
func DefaultSNSHost(host string) bool { return snsHost.MatchString(host) }

// maxSNSBody caps what ParseSNS reads. SNS messages are at most 256 KB, plus the envelope.
const maxSNSBody = 1 << 20

// ParseSNS reads one SNS delivery from r and checks its signature with v before trusting
// anything in it. A Notification returns its S3 changes in Changes. A subscription or
// unsubscription message returns the message with ErrSubscriptionConfirmation. A message that
// fails the check returns ErrBadSignature, and a body that isn't an SNS message returns
// mounts.ErrInvalidArgument.
func (v *SNSVerifier) ParseSNS(r *http.Request) (*SNSMessage, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSNSBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSNSBody {
		return nil, fmt.Errorf("%w: SNS body over %d bytes", mounts.ErrInvalidArgument, maxSNSBody)
	}
	var raw struct {
		Type             string `json:"Type"`
		MessageID        string `json:"MessageId"`
		TopicArn         string `json:"TopicArn"`
		Subject          string `json:"Subject"`
		Message          string `json:"Message"`
		Timestamp        string `json:"Timestamp"`
		SubscribeURL     string `json:"SubscribeURL"`
		Token            string `json:"Token"`
		SignatureVersion string `json:"SignatureVersion"`
		Signature        string `json:"Signature"`
		SigningCertURL   string `json:"SigningCertURL"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Type == "" {
		return nil, fmt.Errorf("%w: not an SNS message", mounts.ErrInvalidArgument)
	}
	m := &SNSMessage{Type: raw.Type, MessageID: raw.MessageID, TopicArn: raw.TopicArn, Subject: raw.Subject,
		Message: raw.Message, Timestamp: raw.Timestamp, SubscribeURL: raw.SubscribeURL, Token: raw.Token,
		signatureVersion: raw.SignatureVersion, signature: raw.Signature, signingCertURL: raw.SigningCertURL}
	if err := v.verify(r.Context(), m); err != nil {
		return nil, err
	}
	switch m.Type {
	case "Notification":
		if m.Changes, err = ParseEvent([]byte(m.Message)); err != nil {
			return nil, err
		}
		return m, nil
	case "SubscriptionConfirmation", "UnsubscribeConfirmation":
		return m, ErrSubscriptionConfirmation
	}
	return nil, fmt.Errorf("%w: SNS message type %q", mounts.ErrInvalidArgument, m.Type)
}

// ConfirmSubscription accepts the subscription m asks about, by fetching its SubscribeURL. It
// refuses a URL whose host isn't SNS's, so a forged confirmation can't make the server fetch an
// address of the sender's choosing.
func (v *SNSVerifier) ConfirmSubscription(ctx context.Context, m *SNSMessage) error {
	u, err := v.snsURL(m.SubscribeURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := v.client().Do(req)
	if err != nil {
		return fmt.Errorf("%w: confirming the SNS subscription: %v", mounts.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%w: SNS answered %d to the confirmation", mounts.ErrUnavailable, resp.StatusCode)
	}
	return nil
}

func (v *SNSVerifier) client() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return http.DefaultClient
}

// snsURL returns raw if it is an https URL on an SNS host, and ErrBadSignature otherwise.
func (v *SNSVerifier) snsURL(raw string) (string, error) {
	allow := v.AllowHost
	if allow == nil {
		allow = DefaultSNSHost
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || !allow(u.Hostname()) {
		return "", fmt.Errorf("%w: %q isn't an https URL on an SNS host", ErrBadSignature, raw)
	}
	return u.String(), nil
}

// verify checks m's signature against the certificate at its SigningCertURL, as
// https://docs.aws.amazon.com/sns/latest/dg/sns-verify-signature-of-message.html describes.
func (v *SNSVerifier) verify(ctx context.Context, m *SNSMessage) error {
	var h crypto.Hash
	switch m.signatureVersion {
	case "1":
		h = crypto.SHA1
	case "2":
		h = crypto.SHA256
	default:
		return fmt.Errorf("%w: SignatureVersion %q", ErrBadSignature, m.signatureVersion)
	}
	cert, err := v.cert(ctx, m.signingCertURL)
	if err != nil {
		return err
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: the signing certificate's key isn't RSA", ErrBadSignature)
	}
	sig, err := base64.StdEncoding.DecodeString(m.signature)
	if err != nil {
		return fmt.Errorf("%w: the signature isn't base64", ErrBadSignature)
	}
	var digest []byte
	if h == crypto.SHA1 {
		s := sha1.Sum([]byte(stringToSign(m))) // #nosec G401 -- SignatureVersion 1, see the import
		digest = s[:]
	} else {
		s := sha256.Sum256([]byte(stringToSign(m)))
		digest = s[:]
	}
	if err := rsa.VerifyPKCS1v15(pub, h, digest, sig); err != nil {
		return fmt.Errorf("%w: the signature doesn't match", ErrBadSignature)
	}
	return nil
}

// stringToSign lists m's signed fields, each as its name, a newline, its value and a newline, in
// the order SNS signs them. Subject is signed only when the message has one.
func stringToSign(m *SNSMessage) string {
	var b strings.Builder
	add := func(k, v string) { b.WriteString(k + "\n" + v + "\n") }
	add("Message", m.Message)
	add("MessageId", m.MessageID)
	if m.Type == "Notification" {
		if m.Subject != "" {
			add("Subject", m.Subject)
		}
	} else {
		add("SubscribeURL", m.SubscribeURL)
	}
	add("Timestamp", m.Timestamp)
	if m.Type != "Notification" {
		add("Token", m.Token)
	}
	add("TopicArn", m.TopicArn)
	add("Type", m.Type)
	return b.String()
}

// cert returns the certificate at raw, fetching it once per URL. The host check runs before any
// request, so a forged SigningCertURL never makes the server fetch anything.
func (v *SNSVerifier) cert(ctx context.Context, raw string) (*x509.Certificate, error) {
	u, err := v.snsURL(raw)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	c := v.certs[u]
	v.mu.Unlock()
	if c != nil {
		return c, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching the SNS signing certificate: %v", mounts.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%w: the SNS signing certificate answered %d", mounts.ErrUnavailable, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("%w: reading the SNS signing certificate: %v", mounts.ErrUnavailable, err)
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("%w: the signing certificate isn't PEM", ErrBadSignature)
	}
	if c, err = x509.ParseCertificate(block.Bytes); err != nil {
		return nil, fmt.Errorf("%w: the signing certificate doesn't parse: %v", ErrBadSignature, err)
	}
	v.mu.Lock()
	if v.certs == nil {
		v.certs = map[string]*x509.Certificate{}
	}
	v.certs[u] = c
	v.mu.Unlock()
	return c, nil
}
