package cli

import (
	_ "embed"
	"os"
	"os/exec"
	"strings"

	"sd/internal/config"
)

// measureScript is the user's measure.sh (made portable: dig→getent→python3
// fallback for the display-only resolve), embedded so the single sd binary
// carries it — no separate file to deploy or keep in sync. Requires bash, curl,
// bc, awk on the host (present on pi/optiplex).
//
//go:embed measure.sh
var measureScript string

// cmdMeasure times the HTTPS request breakdown (dns/connect/tls/ttfb/total) for
// a service by shelling out to the embedded measure.sh.
//
//	sd measure <service|fqdn>
//
// It measures whatever the local resolver currently returns — on the LAN that
// is the split-horizon record, so it times the internal path. Read-only. Run it
// from a client (e.g. your workstation) for the client-perceived numbers.
func cmdMeasure(cfgPath string, args []string) int {
	if len(args) < 1 {
		errf("Missing the <service> or <fqdn> to measure.")
		hint("Usage: sd measure <service|fqdn>")
		return 2
	}

	cfg, code := loadExisting(cfgPath, "measure")
	if cfg == nil {
		return code
	}

	name, svc, ok := resolveService(cfg, args[0])
	if !ok {
		errf("No service named %q and no service with fqdn %q in services.yaml.", args[0], args[0])
		return 1
	}
	if svc.Disabled {
		errf("Service %q is disabled — enable it before measuring.", name)
		return 1
	}

	url := "https://" + svc.FQDN + "/"
	cmd := exec.Command("bash", "-s", url)
	cmd.Stdin = strings.NewReader(measureScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		errf("measure script failed for %s: %v", url, err)
		return 1
	}
	return 0
}

// resolveService looks up a service by its name first, then by fqdn.
func resolveService(cfg *config.Config, arg string) (string, config.Service, bool) {
	if svc, ok := cfg.Services[arg]; ok {
		return arg, svc, true
	}
	if name := serviceByFQDN(cfg, arg); name != "" {
		return name, cfg.Services[name], true
	}
	return "", config.Service{}, false
}
