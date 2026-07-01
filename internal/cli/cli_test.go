package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// mkdirs creates host directories inside the temp repo so `host add` (which
// requires the host name to be an existing directory) succeeds.
func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// seed writes a minimal valid services.yaml into dir.
func seed(t *testing.T, dir string) {
	t.Helper()
	content := `hosts:
  resolver: {ip: 192.0.2.1, dir: resolver}
  appbox: {ip: 192.0.2.2, dir: appbox}
domains:
  - example.com
defaults:
  dns_host: resolver
services: {}
`
	if err := os.WriteFile(filepath.Join(dir, configName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun_ApplyMissingConfig(t *testing.T) {
	dir := t.TempDir()
	if code := Run([]string{"-C", dir, "apply"}); code != 1 {
		t.Errorf("apply on missing services.yaml should exit 1, got %d", code)
	}
}

func TestRun_UpdateAndRemoveMissingConfig(t *testing.T) {
	dir := t.TempDir()
	if code := Run([]string{"-C", dir, "update", "service", "foo", "--backend", "a:1"}); code != 1 {
		t.Errorf("update on missing config should exit 1, got %d", code)
	}
	if code := Run([]string{"-C", dir, "remove", "service", "foo"}); code != 1 {
		t.Errorf("remove on missing config should exit 1, got %d", code)
	}
}

func TestRun_AddCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	code := Run([]string{"-C", dir, "add", "service", "docs",
		"--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000"})
	if code != 0 {
		t.Fatalf("valid add should exit 0, got %d", code)
	}
	// Generated outputs exist.
	if _, err := os.Stat(filepath.Join(dir, "appbox", "caddy/data/sites/docs.caddy")); err != nil {
		t.Errorf("caddy site not generated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "resolver", "pihole/data/dnsmasq.d/docs.generated.conf")); err != nil {
		t.Errorf("dns conf not generated: %v", err)
	}
}

func TestRun_AddServiceRefusesUnknownFQDN(t *testing.T) {
	// add must refuse a service whose fqdn matches no defined domain (catches
	// typos like .dl for .dk) and NOT persist it.
	dir := t.TempDir()
	seed(t, dir) // defines example.com + hosts
	code := Run([]string{"-C", dir, "add", "service", "docs",
		"--fqdn", "docs.example.dl", "--host", "appbox", "--backend", "paperless:8000"})
	if code != 1 {
		t.Errorf("add with unmatched fqdn should exit 1, got %d", code)
	}
	if _, ok := load(t, dir).Services["docs"]; ok {
		t.Error("service with unmatched fqdn must not be persisted")
	}
}

func TestRun_AddServiceRefusesUnknownHost(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	code := Run([]string{"-C", dir, "add", "service", "docs",
		"--fqdn", "docs.example.com", "--host", "ghost", "--backend", "paperless:8000"})
	if code != 1 {
		t.Errorf("add with unknown host should exit 1, got %d", code)
	}
	if _, ok := load(t, dir).Services["docs"]; ok {
		t.Error("service with unknown host must not be persisted")
	}
}

func TestRun_AddDuplicateFails(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	args := []string{"-C", dir, "add", "service", "docs",
		"--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000"}
	if code := Run(args); code != 0 {
		t.Fatalf("first add should succeed, got %d", code)
	}
	if code := Run(args); code != 1 {
		t.Errorf("duplicate add should exit 1, got %d", code)
	}
}

func TestRun_SyncReportsSkips(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	mkdirs(t, dir, "resolver", "appbox")
	// Valid fqdn/host/domain (passes add) but a malformed backend — the planner
	// skips it during add's sync tail -> exit 1. (add doesn't validate backend
	// shape, so the skip surfaces at reconcile time.)
	code := Run([]string{"-C", dir, "add", "service", "x",
		"--fqdn", "x.example.com", "--host", "resolver", "--backend", "noport"})
	if code != 1 {
		t.Errorf("add with a skipped entry should exit 1, got %d", code)
	}
}

// --- validation guards added in the UX pass ---

func TestRun_AddMissingFlagsDoesNotPersist(t *testing.T) {
	dir := t.TempDir()
	if code := Run([]string{"-C", dir, "add", "service", "docs"}); code != 2 {
		t.Errorf("add with no flags should exit 2, got %d", code)
	}
	// Crucially: no services.yaml should have been written.
	if _, err := os.Stat(filepath.Join(dir, configName)); !os.IsNotExist(err) {
		t.Error("add with missing flags must not create services.yaml")
	}
}

func TestRun_AddPartialFlagsRejected(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	if code := Run([]string{"-C", dir, "add", "service", "docs", "--fqdn", "docs.example.com"}); code != 2 {
		t.Errorf("add missing --host/--backend should exit 2, got %d", code)
	}
	if _, ok := load(t, dir).Services["docs"]; ok {
		t.Error("partial add must not persist the service")
	}
}

func TestRun_UpdateNoOpRejected(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir)
	Run([]string{"-C", dir, "add", "service", "docs",
		"--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000"})
	if code := Run([]string{"-C", dir, "update", "docs"}); code != 2 {
		t.Errorf("update with no field flags should exit 2, got %d", code)
	}
}

func TestRun_List(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir) // resolver + appbox, dns_host resolver, example.com
	mkdirs(t, dir, "resolver", "appbox")
	Run([]string{"-C", dir, "add", "service", "docs",
		"--fqdn", "docs.example.com", "--host", "appbox", "--backend", "paperless:8000"})

	// list is plain inventory — always exits 0, regardless of service validity.
	if code := Run([]string{"-C", dir, "list"}); code != 0 {
		t.Errorf("list should exit 0 (plain inventory), got %d", code)
	}
}

func TestRun_ListMissingConfig(t *testing.T) {
	dir := t.TempDir()
	if code := Run([]string{"-C", dir, "list"}); code != 1 {
		t.Errorf("list with no services.yaml should exit 1, got %d", code)
	}
}
