package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"shd/internal/plan"
)

// cmdVerify checks live DNS resolution for every valid service, per design
// §13. For each service it runs two independent checks:
//
//	pihole (in-container): docker exec pihole dig +short {A,AAAA} <fqdn> @127.0.0.1
//	    asserts A == the service host IP AND AAAA == :: (suppressed). Catches
//	    reload-didn't-happen and dir-not-sourced.
//	client (host view):    getent hosts <fqdn>
//	    asserts the system resolver (= pihole, since all machines use it)
//	    returns the host IP. Catches an overridden record or a leak past
//	    split-horizon. Uses getent (always on Debian) so no dnsutils/dig is
//	    needed on the host.
//
// Meaningful only on/near the resolver: the in-container dig needs the pihole
// container; the client check needs the machine to resolve via pihole. It is
// not expected to pass from a stock laptop checkout (§13 is a post-deploy,
// on-host step).
func cmdVerify(cfgPath string, args []string) int {
	cfg, code := loadExisting(cfgPath, "verify")
	if cfg == nil {
		return code
	}
	p := plan.Build(cfg)

	// Verify only real services (drop synthetic @domain: TLS owners).
	var services []string
	for _, k := range p.Valid() {
		if !plan.IsDomainOwner(k) {
			services = append(services, k)
		}
	}
	if len(services) == 0 {
		fmt.Println("No valid services to verify.")
		return 0
	}

	failures := 0
	for _, name := range services {
		svc := cfg.Services[name]
		hostIP := cfg.Hosts[svc.Host].IP // svc is valid, so host exists with an IP
		fmt.Printf("%s (%s -> %s):\n", name, svc.FQDN, hostIP)
		if !checkPihole(svc.FQDN, hostIP) {
			failures++
		}
		if !checkClient(svc.FQDN, hostIP) {
			failures++
		}
	}

	fmt.Println()
	if failures > 0 {
		errf("%d resolution %s failed.", failures, plural(failures, "check"))
		return 1
	}
	fmt.Printf("All %d services resolve correctly.\n", len(services))
	return 0
}

// checkPihole runs the in-container dig for A and AAAA and asserts
// A == wantIP and AAAA == :: (or absent).
func checkPihole(fqdn, wantIP string) bool {
	a, aErr := run("docker", "exec", "pihole", "dig", "+short", "A", fqdn, "@127.0.0.1")
	aaaa, aaaaErr := run("docker", "exec", "pihole", "dig", "+short", "AAAA", fqdn, "@127.0.0.1")
	if aErr != nil || aaaaErr != nil {
		fmt.Printf("  ✗ pihole   could not query (run on the resolver host with the pihole container)\n")
		return false
	}
	if a == wantIP && (aaaa == "::" || aaaa == "") {
		fmt.Printf("  ✓ pihole   A=%s AAAA=%s\n", a, orNone(aaaa))
		return true
	}
	fmt.Printf("  ✗ pihole   A=%s (want %s)  AAAA=%s (want ::)\n", orNone(a), wantIP, orNone(aaaa))
	return false
}

// checkClient resolves via getent (the host's real resolver path = pihole)
// and asserts the returned address is the host IP.
func checkClient(fqdn, wantIP string) bool {
	out, err := run("getent", "hosts", fqdn)
	if err != nil {
		// getent exits non-zero when the name doesn't resolve at all.
		fmt.Printf("  ✗ client   did not resolve (want %s)\n", wantIP)
		return false
	}
	// getent output: "<ip>   <canonical-name> [aliases...]"
	got := firstField(out)
	if got == wantIP {
		fmt.Printf("  ✓ client   %s\n", got)
		return true
	}
	fmt.Printf("  ✗ client   %s (want %s)\n", orNone(got), wantIP)
	return false
}

// run executes a command and returns the first non-empty output line, trimmed.
func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s, nil
		}
	}
	return "", nil
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
