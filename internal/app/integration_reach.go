package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// integration_reach.go: the console half of an integration's Test button
// (design 0019). It asks whether the servers an integration's settings name
// answer at all, from where the console stands. It never sends a credential:
// secrets are references the console does not hold, so a wrong password is
// for the devices' readings to show.

// ReachKind says how an address is tried.
type ReachKind string

const (
	// ReachURL is an http(s) URL: one GET, redirects not followed.
	ReachURL ReachKind = "url"
	// ReachLDAP is an ldap(s) URI: TCP to the port, a TLS handshake for ldaps.
	ReachLDAP ReachKind = "ldap"
	// ReachWazuh is a Wazuh manager host: TCP to events and enrolment.
	ReachWazuh ReachKind = "wazuh"
)

// IntegrationAddresses are the settings that hold a server address, per
// integration key (the Integrations page's), and how to try each. An
// integration without an entry has no server the console can try.
var IntegrationAddresses = map[string][]struct {
	Option string
	Kind   ReachKind
}{
	"netbird":  {{"netbird.managementUrl", ReachURL}},
	"identity": {{"identity.ldapUri", ReachLDAP}},
	"wazuh":    {{"wazuh.manager", ReachWazuh}},
	"openbao":  {{"openbao.address", ReachURL}},
	"osquery":  {{"osquery.server", ReachURL}},
}

// ReachResult is one thing tried.
type ReachResult struct {
	// Target is what was tried, e.g. "manager.example.org:1515".
	Target string
	OK     bool
	// Detail says what answered ("HTTP 200") or what went wrong, in words.
	Detail string
}

// Reacher tries addresses. The zero value works; tests set Roots.
type Reacher struct {
	// Roots verifies TLS; nil means the system's roots, as on a device.
	Roots *x509.CertPool
	// Timeout per try; 0 means five seconds.
	Timeout time.Duration
}

func (r Reacher) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 5 * time.Second
}

// Reach tries one address of the given kind.
func (r Reacher) Reach(ctx context.Context, kind ReachKind, value string) []ReachResult {
	value = strings.TrimSpace(value)
	switch kind {
	case ReachURL:
		return []ReachResult{r.reachURL(ctx, value)}
	case ReachLDAP:
		return []ReachResult{r.reachLDAP(ctx, value)}
	case ReachWazuh:
		if _, _, err := net.SplitHostPort(value); err == nil {
			return []ReachResult{r.reachTCP(ctx, value, false, "")}
		}
		return []ReachResult{
			r.reachTCP(ctx, net.JoinHostPort(value, "1514"), false, ""),
			r.reachTCP(ctx, net.JoinHostPort(value, "1515"), false, ""),
		}
	}
	return []ReachResult{{Target: value, Detail: "no way to try this kind of address"}}
}

func (r Reacher) reachURL(ctx context.Context, raw string) ReachResult {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ReachResult{Target: raw, Detail: "not an http or https address"}
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: r.Roots, MinVersion: tls.VersionTLS12}},
		// A redirect is an answer; following it would test another server.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ReachResult{Target: raw, Detail: err.Error()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ReachResult{Target: raw, Detail: reachFailure(err)}
	}
	_ = resp.Body.Close()
	return ReachResult{Target: raw, OK: true, Detail: "HTTP " + resp.Status}
}

func (r Reacher) reachLDAP(ctx context.Context, raw string) ReachResult {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "ldap" && u.Scheme != "ldaps") || u.Hostname() == "" {
		return ReachResult{Target: raw, Detail: "not an ldap or ldaps address"}
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"ldap": "389", "ldaps": "636"}[u.Scheme]
	}
	return r.reachTCP(ctx, net.JoinHostPort(u.Hostname(), port), u.Scheme == "ldaps", u.Hostname())
}

// reachTCP connects, and with tlsName set also shakes hands, verifying the
// certificate the way a device will.
func (r Reacher) reachTCP(ctx context.Context, addr string, withTLS bool, tlsName string) ReachResult {
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return ReachResult{Target: addr, Detail: reachFailure(err)}
	}
	defer func() { _ = conn.Close() }()
	if !withTLS {
		return ReachResult{Target: addr, OK: true, Detail: "accepts connections"}
	}
	tc := tls.Client(conn, &tls.Config{ServerName: tlsName, RootCAs: r.Roots, MinVersion: tls.VersionTLS12})
	if err := tc.HandshakeContext(ctx); err != nil {
		return ReachResult{Target: addr, Detail: reachFailure(err)}
	}
	return ReachResult{Target: addr, OK: true, Detail: "accepts connections, certificate valid"}
}

// reachFailure words an error the way an operator acts on it. A certificate
// the devices will refuse is the most common mistake, so it is named.
func reachFailure(err error) string {
	var (
		unknown  x509.UnknownAuthorityError
		hostname x509.HostnameError
		invalid  x509.CertificateInvalidError
		verify   *tls.CertificateVerificationError
		dns      *net.DNSError
	)
	switch {
	case errors.As(err, &unknown), errors.As(err, &hostname), errors.As(err, &invalid), errors.As(err, &verify):
		return "certificate refused: " + rootCause(err)
	case errors.As(err, &dns):
		return fmt.Sprintf("name %s does not resolve", dns.Name)
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		return "no answer within the time limit"
	}
	return rootCause(err)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func rootCause(err error) string {
	for {
		u := errors.Unwrap(err)
		if u == nil {
			return err.Error()
		}
		err = u
	}
}
