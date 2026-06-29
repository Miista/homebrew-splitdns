package cli

import (
	"os"
	"path/filepath"
	"testing"

	"shd/internal/config"
)

func load(t *testing.T, dir string) *config.Config {
	t.Helper()
	c, err := config.Load(filepath.Join(dir, configName))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestHostAdd_CreatesMachine(t *testing.T) {
	dir := t.TempDir()
	code := Run([]string{"-C", dir, "host", "add", "resolver", "--ip", "192.0.2.1", "--dir", "resolver"})
	if code != 0 {
		t.Fatalf("host add should exit 0, got %d", code)
	}
	c := load(t, dir)
	if c.Machines["resolver"].IP != "192.0.2.1" || c.Machines["resolver"].Dir != "resolver" {
		t.Errorf("machine not stored: %+v", c.Machines["resolver"])
	}
}

func TestHostAdd_RequiresIPAndDir(t *testing.T) {
	dir := t.TempDir()
	if code := Run([]string{"-C", dir, "host", "add", "resolver", "--ip", "192.0.2.1"}); code == 0 {
		t.Error("host add without --dir should fail")
	}
}

func TestHostAdd_Duplicate(t *testing.T) {
	dir := t.TempDir()
	Run([]string{"-C", dir, "host", "add", "resolver", "--ip", "192.0.2.1", "--dir", "resolver"})
	if code := Run([]string{"-C", dir, "host", "add", "resolver", "--ip", "1.2.3.4", "--dir", "pi2"}); code != 1 {
		t.Errorf("duplicate host add should exit 1, got %d", code)
	}
}

func TestHostRemove_RefusesWhenReferenced(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir) // defines resolver + appbox, dns_host resolver
	Run([]string{"-C", dir, "add", "docs",
		"--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000"})

	// appbox is host of docs -> refuse.
	if code := Run([]string{"-C", dir, "host", "remove", "appbox"}); code != 1 {
		t.Errorf("removing referenced host should exit 1, got %d", code)
	}
	if _, ok := load(t, dir).Machines["appbox"]; !ok {
		t.Error("referenced host should not have been removed")
	}

	// resolver is the dns_host of docs (via defaults) -> also refuse.
	if code := Run([]string{"-C", dir, "host", "remove", "resolver"}); code != 1 {
		t.Errorf("removing dns_host should exit 1, got %d", code)
	}
}

func TestHostRemove_Unreferenced(t *testing.T) {
	dir := t.TempDir()
	Run([]string{"-C", dir, "host", "add", "spare", "--ip", "10.0.9.9", "--dir", "spare"})
	if code := Run([]string{"-C", dir, "host", "remove", "spare"}); code != 0 {
		t.Errorf("removing unreferenced host should exit 0, got %d", code)
	}
	if _, ok := load(t, dir).Machines["spare"]; ok {
		t.Error("host should have been removed")
	}
}

func TestDomainAdd_CreatesDomain(t *testing.T) {
	dir := t.TempDir()
	code := Run([]string{"-C", dir, "domain", "add", "example.com", "--tls-import", "tls_example_com"})
	if code != 0 {
		t.Fatalf("domain add should exit 0, got %d", code)
	}
	if load(t, dir).Domains["example.com"].TLSImport != "tls_example_com" {
		t.Error("domain not stored")
	}
}

func TestDomainRemove_RefusesWhenReferenced(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	Run([]string{"-C", dir, "add", "docs",
		"--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000"})

	if code := Run([]string{"-C", dir, "domain", "remove", "example.com"}); code != 1 {
		t.Errorf("removing referenced domain should exit 1, got %d", code)
	}
	if _, ok := load(t, dir).Domains["example.com"]; !ok {
		t.Error("referenced domain should not have been removed")
	}
}

// End-to-end bootstrap entirely via CLI: host -> domain -> service -> sync.
func TestBootstrap_ViaCLIOnly(t *testing.T) {
	dir := t.TempDir()
	steps := [][]string{
		{"-C", dir, "host", "add", "appbox", "--ip", "192.0.2.2", "--dir", "appbox"},
		{"-C", dir, "host", "add", "resolver", "--ip", "192.0.2.1", "--dir", "resolver"},
		{"-C", dir, "domain", "add", "example.com", "--tls-import", "tls_example_com"},
		{"-C", dir, "add", "docs", "--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000", "--dns-host", "resolver"},
	}
	for _, s := range steps {
		if code := Run(s); code != 0 {
			t.Fatalf("step %v exited %d", s, code)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "appbox", "caddy/data/sites/docs.caddy")); err != nil {
		t.Errorf("bootstrap should produce a generated caddy site: %v", err)
	}
}
