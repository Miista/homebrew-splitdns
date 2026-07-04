package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"sort"
)

// cmdList prints the declared inventory — hosts, domains, services. It is plain
// inventory, not a status view: no per-service validity checks. The one thing
// it does flag is the global can't-sync blocker (no dns_host), as a prominent
// warning BEFORE the inventory. Read-only.
//
// By default the Services section is filtered to those running on THIS host
// (matched by local IP); --all shows every service regardless of host.
func cmdList(cfgPath string, args []string) int {
	cfg, code := loadExisting(cfgPath, "list")
	if cfg == nil {
		return code
	}

	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	all := fs.Bool("all", false, "show services on every host, not just this one")
	fs.BoolVar(all, "a", false, "alias for --all")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Which host are we on? If we can't tell (IP matches no host), fall back to
	// showing everything rather than an empty list.
	self := localHost(cfg)
	filtered := !*all && self != ""

	// Big fat warning first: without a dns_host, nothing can sync.
	if cfg.Defaults.DNSHost == "" {
		fmt.Println(warn + " No dns_host set — services cannot sync.")
		fmt.Println("   Set the resolver host:  splitdns set dns-host <name>")
		fmt.Println()
	}

	fmt.Printf("%s== Hosts (%d) ==%s\n", boldOn, len(cfg.Hosts), boldOff)
	for _, name := range sortedKeysOf(cfg.Hosts) {
		marker := ""
		if name == cfg.Defaults.DNSHost {
			marker = "  (dns_host)"
		}
		fmt.Printf("  %-12s %s%s\n", name, cfg.Hosts[name].IP, marker)
	}

	fmt.Printf("\n%s== Domains (%d) ==%s\n", boldOn, len(cfg.Domains), boldOff)
	for _, name := range sortedKeysOf(cfg.Domains) {
		fmt.Printf("  %s\n", name)
	}

	// Gather the services to show, applying the this-host filter unless --all.
	var svcNames []string
	for _, name := range sortedKeysOf(cfg.Services) {
		if filtered && cfg.Services[name].Host != self {
			continue
		}
		svcNames = append(svcNames, name)
	}

	if filtered {
		fmt.Printf("\n%s== Services on %s (%d of %d) ==%s\n", boldOn, self, len(svcNames), len(cfg.Services), boldOff)
	} else {
		fmt.Printf("\n%s== Services (%d) ==%s\n", boldOn, len(svcNames), boldOff)
	}
	for _, name := range svcNames {
		svc := cfg.Services[name]
		if svc.Disabled {
			fmt.Printf("  %-12s %s -> %s  (%s)  [disabled]\n", name, svc.FQDN, svc.Host, svc.Backend)
		} else {
			fmt.Printf("  %-12s %s -> %s  (%s)\n", name, svc.FQDN, svc.Host, svc.Backend)
		}
	}
	if filtered && len(svcNames) < len(cfg.Services) {
		fmt.Printf("  (%d on other hosts hidden — use --all to show)\n", len(cfg.Services)-len(svcNames))
	}

	repoRoot := filepath.Dir(cfgPath)
	reportDrift(detectDrift(repoRoot, cfg, loadManifest(repoRoot, cfg)))
	return 0
}

func sortedKeysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
