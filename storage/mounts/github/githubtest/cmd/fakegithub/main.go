// Command fakegithub serves githubtest's fake as a GitHub Enterprise Server built from a seed file,
// for an exercise that runs an app outside the Go test process, such as one in a container.
//
//	go run github.com/panyam/goapplib/storage/mounts/github/githubtest/cmd/fakegithub \
//	  --seed seed.json --addr 127.0.0.1:7070 --listen 0.0.0.0:7071 --base-url http://host.docker.internal:7071
//
// githubtest.Seed describes the seed file. The fake answers the REST API under /api/v3 as a GHES
// host does, and sign-in and raw files at its root. --addr serves the control requests that
// githubtest.Seeded.Control describes: GET / names the fake's addresses, the tags' commits and the
// App's key and installations, POST /approve approves a device code, and POST /push commits and
// delivers a push.
//
// --listen and --base-url set where the fake listens and the origin of every URL it hands out, as
// githubtest.WithListener and WithBaseURL do. --hook-ca names a CA file the fake trusts when it
// delivers a webhook over TLS.
//
// --tls-cert and --tls-key name a PEM certificate and its key. With both, the fake serves HTTPS on
// --listen, and the base URL defaults to https. The certificate has to name the address the app
// dials. An app outside loopback needs this, since oneauth's SetEnterpriseURL refuses a plain-HTTP
// Enterprise host there. The control address on --addr stays plain HTTP.
//
// It is a test tool. Nothing in it is safe to expose beyond a test network.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr, nil); err != nil {
		log.Fatal(err)
	}
}

// process stands in for the testing.TB that githubtest.New takes. The fake only registers a
// cleanup and reports a bad option, and run closes the fake itself.
type process struct{ testing.TB }

func (process) Helper()                   {}
func (process) Cleanup(func())            {}
func (process) Fatalf(f string, a ...any) { panic(fmt.Sprintf(f, a...)) }

// run serves the fake and its control address until ctx ends. It calls ready, when set, with the
// control address once both are up.
func run(ctx context.Context, args []string, stderr io.Writer, ready func(control string)) (err error) {
	fs := flag.NewFlagSet("fakegithub", flag.ContinueOnError)
	fs.SetOutput(stderr)
	seed := fs.String("seed", "", "the seed file (see githubtest.Seed)")
	addr := fs.String("addr", "127.0.0.1:0", "where to serve the control requests")
	listen := fs.String("listen", "", "where the fake listens; empty means a fresh 127.0.0.1 port")
	baseURL := fs.String("base-url", "", "the origin of every URL the fake hands out; empty means its listen address")
	hookCA := fs.String("hook-ca", "", "a CA file the fake trusts when it delivers a webhook over TLS")
	certFile := fs.String("tls-cert", "", "a PEM certificate the fake serves HTTPS with, naming the address the app dials")
	keyFile := fs.String("tls-key", "", "the PEM key for --tls-cert")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *seed == "" {
		return errors.New("fakegithub: --seed is required")
	}
	sd, err := githubtest.LoadSeed(*seed)
	if err != nil {
		return err
	}

	var opts []githubtest.Option
	if *listen != "" {
		l, err := net.Listen("tcp", *listen)
		if err != nil {
			return err
		}
		opts = append(opts, githubtest.WithListener(l))
	}
	if *baseURL != "" {
		opts = append(opts, githubtest.WithBaseURL(*baseURL))
	}
	if (*certFile == "") != (*keyFile == "") {
		return errors.New("fakegithub: HTTPS needs both --tls-cert and --tls-key")
	}
	if *certFile != "" {
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			return fmt.Errorf("fakegithub: %w", err)
		}
		opts = append(opts, githubtest.WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}))
	}
	if *hookCA != "" {
		pem, err := os.ReadFile(*hookCA) // #nosec G304 -- the CA file the caller names
		if err != nil {
			return err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("fakegithub: --hook-ca %s holds no certificate", *hookCA)
		}
		opts = append(opts, githubtest.WithHookClient(&http.Client{Timeout: 30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}))
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("fakegithub: %v", r)
		}
	}()
	gh := githubtest.New(process{}, opts...)
	defer gh.Close()
	e, err := sd.Apply(gh)
	if err != nil {
		return err
	}
	// A device-flow client polls at the fake's interval, so a short one keeps a sign-in quick.
	gh.SetDeviceInterval(200 * time.Millisecond)

	cl, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: e.Control()}
	fmt.Fprintf(stderr, "fakegithub: an Enterprise Server at %s (listening on %s), answering at http://%s\n", gh.URL, gh.ListenURL(), cl.Addr())
	if ready != nil {
		ready("http://" + cl.Addr().String())
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(cl); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
