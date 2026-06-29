package sync

import (
	"os"
	"path/filepath"
	"testing"

	"shd/internal/manifest"
	"shd/internal/plan"
)

func newEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	root := t.TempDir()
	mf, _ := manifest.Load(filepath.Join(root, "shd-manifest.yaml"))
	return &Engine{RepoRoot: root, Manifest: mf}, root
}

func planWith(files map[string][]plan.File) *plan.Plan {
	return &plan.Plan{Files: files, Skipped: map[string]string{}, Total: len(files)}
}

func TestReconcile_WritesValidEntries(t *testing.T) {
	eng, root := newEngine(t)
	p := planWith(map[string][]plan.File{
		"docs": {{Path: "resolver/docs.conf", Content: "dns"}, {Path: "appbox/docs.caddy", Content: "caddy"}},
	})
	res, err := eng.Reconcile(p, Incremental)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Written) != 2 {
		t.Errorf("expected 2 written, got %v", res.Written)
	}
	got, _ := os.ReadFile(filepath.Join(root, "resolver/docs.conf"))
	if string(got) != "dns" {
		t.Errorf("content mismatch: %q", got)
	}
	if eng.Manifest.Files("docs") == nil {
		t.Error("manifest should track docs after write")
	}
}

func TestReconcile_IncrementalNeverDeletes(t *testing.T) {
	eng, root := newEngine(t)
	// First sync creates docs.
	eng.Reconcile(planWith(map[string][]plan.File{
		"docs": {{Path: "resolver/docs.conf", Content: "dns"}},
	}), Incremental)

	// docs removed from desired plan, but incremental must NOT delete it.
	_, err := eng.Reconcile(planWith(map[string][]plan.File{}), Incremental)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "resolver/docs.conf")); err != nil {
		t.Error("incremental sync must not delete orphaned files")
	}
}

func TestReconcile_CompleteGCsOrphans(t *testing.T) {
	eng, root := newEngine(t)
	eng.Reconcile(planWith(map[string][]plan.File{
		"docs": {{Path: "resolver/docs.conf", Content: "dns"}},
	}), Incremental)

	// docs gone from plan; complete should GC it.
	_, err := eng.Reconcile(planWith(map[string][]plan.File{}), Complete)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "resolver/docs.conf")); !os.IsNotExist(err) {
		t.Error("complete sync should delete orphaned manifest-tracked file")
	}
	if eng.Manifest.Files("docs") != nil {
		t.Error("docs should be dropped from manifest after GC")
	}
}

// The core safety invariant: complete-mode GC must never touch a file that is
// not tracked in the manifest (e.g. a hand-written lan.conf).
func TestReconcile_CompleteNeverTouchesUntrackedFiles(t *testing.T) {
	eng, root := newEngine(t)
	hand := filepath.Join(root, "resolver", "lan.conf")
	if err := os.MkdirAll(filepath.Dir(hand), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(hand, []byte("hand-written"), 0o644)

	eng.Reconcile(planWith(map[string][]plan.File{
		"docs": {{Path: "resolver/docs.conf", Content: "dns"}},
	}), Incremental)
	// Remove docs and GC.
	eng.Reconcile(planWith(map[string][]plan.File{}), Complete)

	if _, err := os.Stat(hand); err != nil {
		t.Error("untracked hand-written file must survive GC")
	}
}

func TestRemoveService(t *testing.T) {
	eng, root := newEngine(t)
	eng.Reconcile(planWith(map[string][]plan.File{
		"docs": {{Path: "resolver/docs.conf", Content: "dns"}, {Path: "appbox/docs.caddy", Content: "c"}},
	}), Incremental)

	res, err := eng.RemoveService("docs")
	if err != nil {
		t.Fatalf("RemoveService: %v", err)
	}
	if len(res.Deleted) != 2 {
		t.Errorf("expected 2 deleted, got %v", res.Deleted)
	}
	if _, err := os.Stat(filepath.Join(root, "resolver/docs.conf")); !os.IsNotExist(err) {
		t.Error("removed service file should be gone")
	}
	if eng.Manifest.Files("docs") != nil {
		t.Error("docs should be dropped from manifest")
	}
}
