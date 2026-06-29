package cli

import (
	"flag"
	"fmt"
	"strings"

	"shd/internal/config"
)

// cmdHost handles `host add|remove`. The command noun matches the schema key
// `hosts:`. These commands mutate the YAML but do NOT write generated files:
// hosts/domains have no per-host output of their own, and a run that changes
// them is followed by sync to regenerate affected services.
func cmdHost(cfgPath string, args []string) int {
	if len(args) < 1 {
		errf("host requires a subcommand.")
		hint("Usage: shd host add|remove <name> ...")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return hostAdd(cfgPath, rest)
	case "remove":
		return hostRemove(cfgPath, rest)
	default:
		errf("Unknown host subcommand %q — expected add or remove.", sub)
		return 2
	}
}

func hostAdd(cfgPath string, args []string) int {
	name, args, ok := leadingName(args)
	if !ok {
		errf("host add requires a <name>.")
		hint("Usage: shd host add <name> --ip <ip> --dir <dir>")
		return 2
	}
	fs := flag.NewFlagSet("host add", flag.ContinueOnError)
	ip := fs.String("ip", "", "host LAN IP (required)")
	dir := fs.String("dir", "", "repo directory for this host (required)")
	dnsmasqDir := fs.String("dnsmasq-dir", "", "override dnsmasq output dir")
	caddyDir := fs.String("caddy-sites-dir", "", "override caddy sites dir")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var missing []string
	if *ip == "" {
		missing = append(missing, "--ip")
	}
	if *dir == "" {
		missing = append(missing, "--dir")
	}
	if len(missing) > 0 {
		errf("host add is missing required flag(s): %s.", strings.Join(missing, ", "))
		hint("Usage: shd host add %s --ip <ip> --dir <dir>", name)
		return 2
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if _, exists := cfg.Hosts[name]; exists {
		errf("Host %q already exists.", name)
		return 1
	}
	cfg.Hosts[name] = config.Host{
		IP: *ip, Dir: *dir, DnsmasqDir: *dnsmasqDir, CaddySitesDir: *caddyDir,
	}

	// First host bootstraps the default dns_host so services resolve without a
	// manual edit; tell the user and how to change it.
	setDefault := cfg.Defaults.DNSHost == ""
	if setDefault {
		cfg.Defaults.DNSHost = name
	}
	if err := cfg.Save(); err != nil {
		errf("%v", err)
		return 1
	}
	fmt.Printf("Added host %q (%s, dir %s).\n", name, *ip, *dir)
	if setDefault {
		fmt.Printf("Set default dns_host to %q. Change it with: shd dns-host set <name>\n", name)
	}
	return 0
}

func hostRemove(cfgPath string, args []string) int {
	if len(args) < 1 {
		errf("host remove requires a <name>.")
		return 2
	}
	name := args[0]

	cfg, code := loadExisting(cfgPath, "remove a host from")
	if cfg == nil {
		return code
	}
	if _, exists := cfg.Hosts[name]; !exists {
		errf("Host %q does not exist.", name)
		return 1
	}
	if users := cfg.ServicesUsingHost(name); len(users) > 0 {
		errf("Host %q is still referenced by %d service(s): %s.", name, len(users), strings.Join(users, ", "))
		hint("Reassign or remove those services first.")
		return 1
	}
	delete(cfg.Hosts, name)
	if err := cfg.Save(); err != nil {
		errf("%v", err)
		return 1
	}
	fmt.Printf("Removed host %q.\n", name)
	return 0
}

// cmdDomain handles `domain add|remove`.
func cmdDomain(cfgPath string, args []string) int {
	if len(args) < 1 {
		errf("domain requires a subcommand.")
		hint("Usage: shd domain add|remove <name> ...")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return domainAdd(cfgPath, rest)
	case "remove":
		return domainRemove(cfgPath, rest)
	default:
		errf("Unknown domain subcommand %q — expected add or remove.", sub)
		return 2
	}
}

func domainAdd(cfgPath string, args []string) int {
	name, args, ok := leadingName(args)
	if !ok {
		errf("domain add requires a <name>.")
		hint("Usage: shd domain add <name> --tls-import <snippet>")
		return 2
	}
	fs := flag.NewFlagSet("domain add", flag.ContinueOnError)
	tlsImport := fs.String("tls-import", "", "Caddy tls snippet name, e.g. tls_example_com (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *tlsImport == "" {
		errf("domain add is missing required flag: --tls-import.")
		hint("Usage: shd domain add %s --tls-import <snippet>", name)
		return 2
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if _, exists := cfg.Domains[name]; exists {
		errf("Domain %q already exists.", name)
		return 1
	}
	cfg.Domains[name] = config.Domain{TLSImport: *tlsImport}
	if err := cfg.Save(); err != nil {
		errf("%v", err)
		return 1
	}
	fmt.Printf("Added domain %q (imports %s).\n", name, *tlsImport)
	return 0
}

func domainRemove(cfgPath string, args []string) int {
	if len(args) < 1 {
		errf("domain remove requires a <name>.")
		return 2
	}
	name := args[0]

	cfg, code := loadExisting(cfgPath, "remove a domain from")
	if cfg == nil {
		return code
	}
	if _, exists := cfg.Domains[name]; !exists {
		errf("Domain %q does not exist.", name)
		return 1
	}
	if users := cfg.ServicesUsingDomain(name); len(users) > 0 {
		errf("Domain %q is still referenced by %d service(s): %s.", name, len(users), strings.Join(users, ", "))
		hint("Reassign or remove those services first.")
		return 1
	}
	delete(cfg.Domains, name)
	if err := cfg.Save(); err != nil {
		errf("%v", err)
		return 1
	}
	fmt.Printf("Removed domain %q.\n", name)
	return 0
}

// cmdDNSHost handles `dns-host set <name>` — sets defaults.dns_host, the
// host whose dnsmasq receives address= records unless a service overrides
// it. Without this, a CLI-only bootstrap leaves dns_host unset and every
// service is skipped.
func cmdDNSHost(cfgPath string, args []string) int {
	if len(args) < 1 || args[0] != "set" {
		errf("dns-host requires the 'set' subcommand.")
		hint("Usage: shd dns-host set <name>")
		return 2
	}
	if len(args) < 2 {
		errf("dns-host set requires a <name>.")
		hint("Usage: shd dns-host set <name>")
		return 2
	}
	name := args[1]

	cfg, code := loadExisting(cfgPath, "set the dns-host in")
	if cfg == nil {
		return code
	}
	if _, exists := cfg.Hosts[name]; !exists {
		errf("Host %q does not exist — add it first with: shd host add %s --ip <ip> --dir <dir>", name, name)
		return 1
	}
	cfg.Defaults.DNSHost = name
	if err := cfg.Save(); err != nil {
		errf("%v", err)
		return 1
	}
	fmt.Printf("Set default dns_host to %q.\n", name)
	return 0
}
