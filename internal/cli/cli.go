// Package cli parses commands and wires the engine. add/update/remove mutate
// the YAML then call the shared sync engine; they contain no file-writing
// logic of their own (design §6 single-writer invariant).
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"shd/internal/config"
	"shd/internal/manifest"
	"shd/internal/plan"
	syncpkg "shd/internal/sync"
)

const (
	configName   = "services.yaml"
	manifestName = "shd-manifest.yaml"
)

// Version is the build version, overridden at release time via
// -ldflags "-X shd/internal/cli.Version=...".
var Version = "dev"

// Run executes the CLI. Returns a process exit code (design §8: non-zero if
// any entry was skipped or an error occurred).
func Run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	// Operate on the current directory by default; -C <dir> changes it
	// (git-style). Strip a leading -C/<dir> before dispatching.
	repoRoot := "."
	if args[0] == "-C" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "-C requires a directory")
			return 2
		}
		repoRoot = args[1]
		args = args[2:]
		if len(args) < 1 {
			usage()
			return 2
		}
	}
	cfgPath := filepath.Join(repoRoot, configName)

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("shd", Version)
		return 0
	case "add":
		return cmdAdd(repoRoot, cfgPath, rest)
	case "update":
		return cmdUpdate(repoRoot, cfgPath, rest)
	case "remove":
		return cmdRemove(repoRoot, cfgPath, rest)
	case "host":
		return cmdHost(cfgPath, rest)
	case "domain":
		return cmdDomain(cfgPath, rest)
	case "sync":
		return cmdSync(repoRoot, cfgPath, rest)
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		return 2
	}
}

func cmdAdd(repoRoot, cfgPath string, args []string) int {
	// Usage puts <service> before flags; Go's flag pkg stops at the first
	// positional, so split it off first.
	name, args, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "add: missing <service> name")
		return 2
	}
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fqdn := fs.String("fqdn", "", "service fqdn")
	host := fs.String("host", "", "machine that runs the service")
	backend := fs.String("backend", "", "reverse_proxy upstream name:port")
	dnsHost := fs.String("dns-host", "", "optional dns_host override")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	if _, exists := cfg.Services[name]; exists {
		fmt.Fprintf(os.Stderr, "add: service %q already exists\n", name)
		return 1
	}
	for n, s := range cfg.Services {
		if s.FQDN == *fqdn {
			fmt.Fprintf(os.Stderr, "add: fqdn %q already used by %q\n", *fqdn, n)
			return 1
		}
	}
	cfg.Services[name] = config.Service{FQDN: *fqdn, Host: *host, Backend: *backend, DNSHost: *dnsHost}
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	return runSync(repoRoot, cfg, syncpkg.Incremental)
}

func cmdUpdate(repoRoot, cfgPath string, args []string) int {
	name, args, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "update: missing <service> name")
		return 2
	}
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fqdn := fs.String("fqdn", "", "service fqdn")
	host := fs.String("host", "", "machine that runs the service")
	backend := fs.String("backend", "", "reverse_proxy upstream name:port")
	dnsHost := fs.String("dns-host", "", "dns_host override")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, code := loadExisting(cfgPath, "update")
	if cfg == nil {
		return code
	}
	svc, exists := cfg.Services[name]
	if !exists {
		fmt.Fprintf(os.Stderr, "update: service %q does not exist\n", name)
		return 1
	}
	// Only override fields that were explicitly set on the command line.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "fqdn":
			svc.FQDN = *fqdn
		case "host":
			svc.Host = *host
		case "backend":
			svc.Backend = *backend
		case "dns-host":
			svc.DNSHost = *dnsHost
		}
	})
	cfg.Services[name] = svc
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	return runSync(repoRoot, cfg, syncpkg.Incremental)
}

func cmdRemove(repoRoot, cfgPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "remove: missing <service> name")
		return 2
	}
	name := args[0]

	cfg, code := loadExisting(cfgPath, "remove")
	if cfg == nil {
		return code
	}
	if _, exists := cfg.Services[name]; !exists {
		fmt.Fprintf(os.Stderr, "remove: service %q does not exist\n", name)
		return 1
	}
	delete(cfg.Services, name)
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}

	mf := loadManifest(repoRoot, cfg)
	eng := &syncpkg.Engine{RepoRoot: repoRoot, Manifest: mf}
	res, err := eng.RemoveService(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	for _, d := range res.Deleted {
		fmt.Printf("deleted %s\n", d)
	}
	fmt.Printf("removed service %q\n", name)
	return 0
}

func cmdSync(repoRoot, cfgPath string, args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	incremental := fs.Bool("incremental", false, "write/update only, never delete (default)")
	complete := fs.Bool("complete", false, "incremental plus GC of orphaned tracked files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *incremental && *complete {
		fmt.Fprintln(os.Stderr, "sync: --incremental and --complete are mutually exclusive")
		return 2
	}
	mode := syncpkg.Incremental
	if *complete {
		mode = syncpkg.Complete
	}

	cfg, code := loadExisting(cfgPath, "sync")
	if cfg == nil {
		return code
	}
	return runSync(repoRoot, cfg, mode)
}

// runSync builds the plan, reconciles, prints a summary, and returns the exit
// code per design §8.
func runSync(repoRoot string, cfg *config.Config, mode syncpkg.Mode) int {
	p := plan.Build(cfg)
	mf := loadManifest(repoRoot, cfg)
	eng := &syncpkg.Engine{RepoRoot: repoRoot, Manifest: mf}

	res, err := eng.Reconcile(p, mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}

	for _, w := range res.Written {
		fmt.Printf("wrote %s\n", w)
	}
	for _, d := range res.Deleted {
		fmt.Printf("deleted %s\n", d)
	}

	synced := len(res.Synced)
	total := res.Total
	fmt.Printf("synced %d/%d services", synced, total)
	if len(res.Skipped) > 0 {
		fmt.Printf("; %d skipped:", len(res.Skipped))
		for _, name := range sortedSkip(res.Skipped) {
			fmt.Printf(" %s (%s)", name, res.Skipped[name])
		}
	}
	fmt.Println()

	if len(res.Skipped) > 0 {
		return 1
	}
	return 0
}

// loadManifest loads the manifest, rebuilding it if unparseable (design §5/§7).
func loadManifest(repoRoot string, cfg *config.Config) *manifest.Manifest {
	mfPath := filepath.Join(repoRoot, manifestName)
	mf, ok := manifest.Load(mfPath)
	if !ok {
		fmt.Fprintln(os.Stderr, "warning: manifest unparseable or partial — rebuilding from services.yaml")
		mf = manifest.Rebuild(mfPath, repoRoot, plan.Build(cfg))
	}
	return mf
}

func sortedSkip(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// simple insertion sort to avoid importing sort here twice; small N
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// loadExisting loads services.yaml for commands that read existing state
// (sync, update, remove). A missing file is treated as user error with a
// guiding message rather than an empty config, so a new user in the wrong
// directory — or one who hasn't created any service yet — is told what to do.
// add does NOT use this: it is allowed to create services.yaml.
func loadExisting(cfgPath, command string) (*config.Config, int) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return nil, 1
	}
	if !cfg.Exists {
		fmt.Fprintf(os.Stderr, "no %s in this directory — nothing to %s.\n", configName, command)
		fmt.Fprintln(os.Stderr, "create your first service with:")
		fmt.Fprintln(os.Stderr, "  shd add <name> --fqdn <fqdn> --host <machine> --backend <name:port>")
		fmt.Fprintln(os.Stderr, "(or run from the repo root, or use -C <dir>)")
		return nil, 1
	}
	return cfg, 0
}

// leadingName splits a leading positional <service> from the remaining flag
// args. Returns ok=false if the first token is missing or looks like a flag.
func leadingName(args []string) (name string, rest []string, ok bool) {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		return "", args, false
	}
	return args[0], args[1:], true
}

func usage() {
	fmt.Fprint(os.Stderr, `shd — generate split-horizon DNS + Caddy site blocks from services.yaml

Operates on services.yaml in the current directory.

Usage:
  shd [-C <dir>] add    <service> --fqdn <f> --host <h> --backend <b> [--dns-host <d>]
  shd [-C <dir>] update <service> [--fqdn ...] [--host ...] [--backend ...] [--dns-host ...]
  shd [-C <dir>] remove <service>
  shd [-C <dir>] sync   [--incremental | --complete]

  shd [-C <dir>] host   add    <name> --ip <ip> --dir <dir> [--dnsmasq-dir <d>] [--caddy-sites-dir <d>]
  shd [-C <dir>] host   remove <name>
  shd [-C <dir>] domain add    <name> --tls-import <snippet>
  shd [-C <dir>] domain remove <name>

  -C <dir>   run as if shd were started in <dir> (default: current dir)

host/domain define the building blocks a service references; remove refuses
while any service still uses the host or domain.
`)
}
