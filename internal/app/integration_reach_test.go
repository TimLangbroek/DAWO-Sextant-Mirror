package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func listen(t *testing.T) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return l.Addr().String(), func() { _ = l.Close() }
}

// closedAddr is an address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	addr, stop := listen(t)
	stop()
	return addr
}

func TestReachAURL(t *testing.T) {
	ctx := context.Background()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/", http.StatusFound)
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer secure.Close()
	roots := x509.NewCertPool()
	roots.AddCert(secure.Certificate())

	// A redirect is an answer; the other server is never asked.
	got := Reacher{}.Reach(ctx, ReachURL, plain.URL)
	if len(got) != 1 || !got[0].OK || got[0].Detail != "HTTP 302 Found" {
		t.Fatalf("plain: %+v", got)
	}
	if got := (Reacher{Roots: roots}).Reach(ctx, ReachURL, secure.URL); !got[0].OK || got[0].Detail != "HTTP 200 OK" {
		t.Fatalf("trusted TLS: %+v", got)
	}
	// The certificate a device would refuse is named as such.
	if got := (Reacher{}).Reach(ctx, ReachURL, secure.URL); got[0].OK || !strings.HasPrefix(got[0].Detail, "certificate refused") {
		t.Fatalf("untrusted TLS: %+v", got)
	}
	if got := (Reacher{}).Reach(ctx, ReachURL, "http://"+closedAddr(t)); got[0].OK {
		t.Fatalf("closed port: %+v", got)
	}
	for _, bad := range []string{"", "ftp://x", "manager.example"} {
		if got := (Reacher{}).Reach(ctx, ReachURL, bad); got[0].OK || got[0].Detail != "not an http or https address" {
			t.Errorf("%q: %+v", bad, got)
		}
	}
}

func TestReachLDAP(t *testing.T) {
	ctx := context.Background()
	open, stop := listen(t)
	defer stop()
	if got := (Reacher{}).Reach(ctx, ReachLDAP, "ldap://"+open); !got[0].OK || got[0].Target != open {
		t.Fatalf("ldap: %+v", got)
	}
	if got := (Reacher{}).Reach(ctx, ReachLDAP, "ldap://"+closedAddr(t)); got[0].OK {
		t.Fatalf("closed ldap: %+v", got)
	}
	if got := (Reacher{}).Reach(ctx, ReachLDAP, "ldap.example.org"); got[0].OK || got[0].Detail != "not an ldap or ldaps address" {
		t.Fatalf("no scheme: %+v", got)
	}

	// ldaps: the handshake is checked, not just the port.
	srv := httptest.NewUnstartedServer(http.NotFoundHandler())
	srv.StartTLS()
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	_, port, _ := net.SplitHostPort(addr)
	uri := "ldaps://127.0.0.1:" + port
	if got := (Reacher{Roots: roots}).Reach(ctx, ReachLDAP, uri); !got[0].OK || !strings.Contains(got[0].Detail, "certificate valid") {
		t.Fatalf("trusted ldaps: %+v", got)
	}
	if got := (Reacher{}).Reach(ctx, ReachLDAP, uri); got[0].OK || !strings.HasPrefix(got[0].Detail, "certificate refused") {
		t.Fatalf("untrusted ldaps: %+v", got)
	}
}

func TestReachAWazuhManager(t *testing.T) {
	ctx := context.Background()
	open, stop := listen(t)
	defer stop()
	if got := (Reacher{}).Reach(ctx, ReachWazuh, open); len(got) != 1 || !got[0].OK {
		t.Fatalf("host:port: %+v", got)
	}
	// A bare host is tried on both ports, each reported.
	got := (Reacher{Timeout: time.Second}).Reach(ctx, ReachWazuh, "127.0.0.1")
	if len(got) != 2 || got[0].Target != "127.0.0.1:1514" || got[1].Target != "127.0.0.1:1515" {
		t.Fatalf("bare host: %+v", got)
	}
}

func TestReachFailureNamesTheCause(t *testing.T) {
	if got := reachFailure(&net.DNSError{Name: "nope.invalid", Err: "no such host"}); got != "name nope.invalid does not resolve" {
		t.Errorf("dns: %q", got)
	}
	if got := reachFailure(context.DeadlineExceeded); got != "no answer within the time limit" {
		t.Errorf("timeout: %q", got)
	}
	if got := reachFailure(&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}); !strings.HasPrefix(got, "certificate refused") {
		t.Errorf("certificate: %q", got)
	}
}
