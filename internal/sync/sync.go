// Package sync is the reconcile engine — the ONLY code that writes or deletes
// generated files (design §6 single-writer invariant). It diffs the desired
// plan against the manifest, writes valid outputs, and (in complete mode)
// GCs orphaned tracked files.
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"shd/internal/config"
	"shd/internal/manifest"
	"shd/internal/plan"
)

// Mode selects reconcile behavior.
type Mode int

const (
	// Incremental writes/updates valid entries and never deletes (default).
	Incremental Mode = iota
	// Complete is incremental plus GC of orphaned manifest-tracked files.
	Complete
)

// Result summarizes a sync run.
type Result struct {
	Written []string // file paths written/updated
	Deleted []string // file paths deleted
	Synced  []string // service names successfully synced
	Skipped map[string]string
	Total   int
}

// Engine reconciles the repo at repoRoot.
type Engine struct {
	RepoRoot string
	Manifest *manifest.Manifest
}

// Reconcile makes generated files match the plan. It mutates and saves the
// manifest. repoRoot-relative paths in the plan are joined onto RepoRoot for
// disk I/O; the manifest stores repo-relative paths.
func (e *Engine) Reconcile(p *plan.Plan, mode Mode) (*Result, error) {
	res := &Result{Skipped: p.Skipped, Total: p.Total}

	// Write valid entries.
	for _, svc := range p.Valid() {
		var paths []string
		for _, f := range p.Files[svc] {
			abs := filepath.Join(e.RepoRoot, f.Path)
			if err := config.AtomicWrite(abs, []byte(f.Content)); err != nil {
				return res, fmt.Errorf("write %s for %s: %w", f.Path, svc, err)
			}
			res.Written = append(res.Written, f.Path)
			paths = append(paths, f.Path)
		}
		e.Manifest.Set(svc, paths)
		res.Synced = append(res.Synced, svc)
	}

	if mode == Complete {
		if err := e.gc(p, res); err != nil {
			return res, err
		}
	}

	sort.Strings(res.Written)
	sort.Strings(res.Deleted)
	sort.Strings(res.Synced)

	if err := e.Manifest.Save(); err != nil {
		return res, fmt.Errorf("save manifest: %w", err)
	}
	return res, nil
}

// gc deletes manifest-tracked files whose backing service has no valid plan
// entry. Never touches non-manifest files (design §5/§6).
func (e *Engine) gc(p *plan.Plan, res *Result) error {
	valid := map[string]bool{}
	for _, s := range p.Valid() {
		valid[s] = true
	}
	for _, svc := range e.Manifest.Services() {
		if valid[svc] {
			continue // still has a valid entry; its files were just rewritten
		}
		if err := e.deleteService(svc, res); err != nil {
			return err
		}
	}
	return nil
}

// deleteService removes all manifest-tracked files for a service and drops it
// from the manifest. Shared by --complete GC and the remove command (§6).
func (e *Engine) deleteService(svc string, res *Result) error {
	for _, rel := range e.Manifest.Files(svc) {
		abs := filepath.Join(e.RepoRoot, rel)
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s for %s: %w", rel, svc, err)
		}
		res.Deleted = append(res.Deleted, rel)
	}
	e.Manifest.Remove(svc)
	return nil
}

// RemoveService deletes one service's tracked files and persists the manifest.
// Used by the remove command (the shared delete primitive, §6).
func (e *Engine) RemoveService(svc string) (*Result, error) {
	res := &Result{Skipped: map[string]string{}}
	if err := e.deleteService(svc, res); err != nil {
		return res, err
	}
	sort.Strings(res.Deleted)
	if err := e.Manifest.Save(); err != nil {
		return res, fmt.Errorf("save manifest: %w", err)
	}
	return res, nil
}
