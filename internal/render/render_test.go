package render

import (
	"strings"
	"testing"
)

func TestDNSRecord(t *testing.T) {
	got := DNSRecord("docs.example.com", "192.0.2.2")
	want := Header + "\n" +
		"local=/docs.example.com/\n" +
		"address=/docs.example.com/192.0.2.2\n" +
		"address=/docs.example.com/::\n"
	if got != want {
		t.Fatalf("DNSRecord mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// The :: vs ::1 distinction is structural (design §4.1): :: suppresses the
// public AAAA; ::1 is an explicit bug.
func TestDNSRecord_SuppressesAAAAWithUnspecified(t *testing.T) {
	got := DNSRecord("x.example.net", "192.0.2.1")
	if !strings.Contains(got, "address=/x.example.net/::\n") {
		t.Errorf("missing AAAA-suppression line: %q", got)
	}
	if strings.Contains(got, "::1") {
		t.Errorf("emitted ::1 (loopback) — must be :: (unspecified): %q", got)
	}
}

func TestCaddySite(t *testing.T) {
	got := CaddySite("docs.example.com", "tls_example_com", "paperless:8000", false, false)
	want := Header + "\n" +
		"docs.example.com {\n" +
		"\timport tls_example_com\n" +
		"\treverse_proxy paperless:8000\n" +
		"}\n"
	if got != want {
		t.Fatalf("CaddySite mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestCaddySite_Auth(t *testing.T) {
	got := CaddySite("docs.example.com", "tls_example_com", "paperless:8000", true, false)
	want := Header + "\n" +
		"docs.example.com {\n" +
		"\timport tls_example_com\n" +
		"\timport auth\n" +
		"\treverse_proxy paperless:8000\n" +
		"}\n"
	if got != want {
		t.Fatalf("CaddySite(auth) mismatch:\n got: %q\nwant: %q", got, want)
	}
	// The import must precede reverse_proxy so the auth check runs first.
	if strings.Index(got, "import auth") > strings.Index(got, "reverse_proxy") {
		t.Errorf("import auth must come before reverse_proxy: %q", got)
	}
}

// The auth backend (the Authelia portal) preserves the inbound X-Forwarded-Host
// via a header_up inside reverse_proxy, so post-login redirects target the
// original service. It is never itself behind auth (auth=false here).
func TestCaddySite_AuthBackend(t *testing.T) {
	got := CaddySite("auth.example.com", "tls_example_com", "authelia:9091", false, true)
	want := Header + "\n" +
		"auth.example.com {\n" +
		"\timport tls_example_com\n" +
		"\treverse_proxy authelia:9091 {\n" +
		"\t\theader_up X-Forwarded-Host {header.X-Forwarded-Host}\n" +
		"\t}\n" +
		"}\n"
	if got != want {
		t.Fatalf("CaddySite(authBackend) mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestAuthSnippet_EmptyStub(t *testing.T) {
	got := AuthSnippet("")
	want := Header + "\n(auth) {\n}\n"
	if got != want {
		t.Fatalf("empty AuthSnippet mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestAuthSnippet_Body(t *testing.T) {
	body := "forward_auth https://auth.example.com {\n\turi /api/authz/forward-auth\n}"
	got := AuthSnippet(body)
	want := Header + "\n(auth) {\n" +
		"\tforward_auth https://auth.example.com {\n" +
		"\t\turi /api/authz/forward-auth\n" +
		"\t}\n" +
		"}\n"
	if got != want {
		t.Fatalf("AuthSnippet body mismatch:\n got: %q\nwant: %q", got, want)
	}
}
