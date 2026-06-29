package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"shd/internal/config"
)

// cmdHost handles `host add|remove`. The schema key stays machines: (design
// §3); only the command noun is "host". These commands mutate the YAML but do
// NOT write generated files: machines/domains have no per-machine output of
// their own, and a run that changes them is followed by sync to regenerate
// affected services.
func cmdHost(cfgPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: shd host add|remove <name> ...")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return hostAdd(cfgPath, rest)
	case "remove":
		return hostRemove(cfgPath, rest)
	default:
		fmt.Fprintf(os.Stderr, "host: unknown subcommand %q (want add|remove)\n", sub)
		return 2
	}
}

func hostAdd(cfgPath string, args []string) int {
	name, args, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "host add: missing <name>")
		return 2
	}
	fs := flag.NewFlagSet("host add", flag.ContinueOnError)
	ip := fs.String("ip", "", "machine LAN IP (required)")
	dir := fs.String("dir", "", "repo directory for this machine (required)")
	dnsmasqDir := fs.String("dnsmasq-dir", "", "override dnsmasq output dir")
	caddyDir := fs.String("caddy-sites-dir", "", "override caddy sites dir")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ip == "" || *dir == "" {
		fmt.Fprintln(os.Stderr, "host add: --ip and --dir are required")
		return 2
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	if _, exists := cfg.Machines[name]; exists {
		fmt.Fprintf(os.Stderr, "host add: host %q already exists\n", name)
		return 1
	}
	cfg.Machines[name] = config.Machine{
		IP: *ip, Dir: *dir, DnsmasqDir: *dnsmasqDir, CaddySitesDir: *caddyDir,
	}
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	fmt.Printf("added host %q (%s, dir %s)\n", name, *ip, *dir)
	return 0
}

func hostRemove(cfgPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "host remove: missing <name>")
		return 2
	}
	name := args[0]

	cfg, code := loadExisting(cfgPath, "remove host from")
	if cfg == nil {
		return code
	}
	if _, exists := cfg.Machines[name]; !exists {
		fmt.Fprintf(os.Stderr, "host remove: host %q does not exist\n", name)
		return 1
	}
	if users := cfg.ServicesUsingHost(name); len(users) > 0 {
		fmt.Fprintf(os.Stderr, "host remove: %q is still referenced by %d service(s): %s\n",
			name, len(users), strings.Join(users, ", "))
		fmt.Fprintln(os.Stderr, "reassign or remove those services first.")
		return 1
	}
	delete(cfg.Machines, name)
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	fmt.Printf("removed host %q\n", name)
	return 0
}

// cmdDomain handles `domain add|remove`.
func cmdDomain(cfgPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: shd domain add|remove <name> ...")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return domainAdd(cfgPath, rest)
	case "remove":
		return domainRemove(cfgPath, rest)
	default:
		fmt.Fprintf(os.Stderr, "domain: unknown subcommand %q (want add|remove)\n", sub)
		return 2
	}
}

func domainAdd(cfgPath string, args []string) int {
	name, args, ok := leadingName(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "domain add: missing <name>")
		return 2
	}
	fs := flag.NewFlagSet("domain add", flag.ContinueOnError)
	tlsImport := fs.String("tls-import", "", "Caddy tls snippet name, e.g. tls_example_com (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *tlsImport == "" {
		fmt.Fprintln(os.Stderr, "domain add: --tls-import is required")
		return 2
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	if _, exists := cfg.Domains[name]; exists {
		fmt.Fprintf(os.Stderr, "domain add: domain %q already exists\n", name)
		return 1
	}
	cfg.Domains[name] = config.Domain{TLSImport: *tlsImport}
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	fmt.Printf("added domain %q (import %s)\n", name, *tlsImport)
	return 0
}

func domainRemove(cfgPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "domain remove: missing <name>")
		return 2
	}
	name := args[0]

	cfg, code := loadExisting(cfgPath, "remove domain from")
	if cfg == nil {
		return code
	}
	if _, exists := cfg.Domains[name]; !exists {
		fmt.Fprintf(os.Stderr, "domain remove: domain %q does not exist\n", name)
		return 1
	}
	if users := cfg.ServicesUsingDomain(name); len(users) > 0 {
		fmt.Fprintf(os.Stderr, "domain remove: %q is still referenced by %d service(s): %s\n",
			name, len(users), strings.Join(users, ", "))
		fmt.Fprintln(os.Stderr, "reassign or remove those services first.")
		return 1
	}
	delete(cfg.Domains, name)
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		return 1
	}
	fmt.Printf("removed domain %q\n", name)
	return 0
}
