package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"hemma/internal/auth"
	"hemma/internal/config"
)

// create — credential generation absorbed from the standalone authcli tool.
// Both subcommands are PRINT-ONLY: they mint credentials with native Go
// crypto and print paste-in snippets. They never write the auth provider's
// config or users database (hand-owned, secret-bearing files), and everything
// provider-specific (digest formats, snippet YAML) lives behind auth.Provider.

// dispatchCreate routes `create app oidc <name> [callback]` and
// `create user <name>`.
func dispatchCreate(cfgPath string, args []string) int {
	if len(args) < 1 {
		errf("Missing what to create — expected app or user.")
		hint("Usage: hemma create app oidc <app_name> [callback_path]  |  hemma create user <username>")
		return 2
	}
	switch args[0] {
	case "app":
		rest := args[1:]
		if len(rest) < 1 || rest[0] != "oidc" {
			errf("Unknown app type — expected oidc.")
			hint("Usage: hemma create app oidc <app_name> [callback_path]")
			return 2
		}
		return cmdCreateAppOIDC(cfgPath, rest[1:])
	case "user":
		return cmdCreateUser(args[1:])
	default:
		errf("Unknown noun %q for create — expected app or user.", args[0])
		hint("Usage: hemma create app oidc <app_name> [callback_path]  |  hemma create user <username>")
		return 2
	}
}

// cmdCreateAppOIDC generates OIDC client credentials for an app and prints
// the provider config snippet. If the app name matches a configured service,
// its real fqdn is used for the redirect URI, and — when the service has auth
// groups — the generated named authorization policy is referenced instead of
// one_factor (see auth.Provider.AccessControl). When the app matches no
// configured service, the redirect host is derived from the repo's configured
// domains (<app>.<first domain alphabetically>); with no domains configured
// there is nothing to derive a host from, so the command refuses with a hint.
func cmdCreateAppOIDC(cfgPath string, args []string) int {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		errf("Missing the <app_name>.")
		hint("Usage: hemma create app oidc <app_name> [callback_path]")
		return 2
	}
	if len(args) > 2 {
		errf("Too many arguments — expected <app_name> [callback_path].")
		return 2
	}
	app := args[0]
	callbackPath := "/CHANGEME"
	if len(args) == 2 {
		callbackPath = args[1]
	}

	fqdn := ""
	policy := "one_factor"
	if cfg, err := config.Load(cfgPath); err == nil && cfg.Exists {
		if svc, ok := cfg.Services[app]; ok {
			fqdn = svc.FQDN
			if len(svc.Auth.Groups) > 0 {
				// The generated access-control artifact names the policy after
				// the service; the client must reference it by that name.
				policy = app
			}
		} else if doms := cfg.DomainNames(); len(doms) > 0 {
			// Unknown app: derive the redirect host from the configured domains
			// (DomainNames is sorted, so this is the first alphabetically).
			fqdn = app + "." + doms[0]
		}
	}
	if fqdn == "" {
		errf("%q matches no configured service and no domains are configured — the redirect URI's host can't be derived.", app)
		hint("Add the service ('hemma add service %s ...') or a domain ('hemma add domain <name>') first.", app)
		return 1
	}

	provider := auth.Default()
	clientID, secret, digest, err := provider.GenerateOIDCClient()
	if err != nil {
		errf("%v", err)
		return 1
	}

	fmt.Printf("Created credentials for client %s\n\n", app)
	fmt.Printf("Client Name: %s\n", app)
	fmt.Printf("Client ID: %s\n", clientID)
	fmt.Printf("Client Secret (%s): %s\n", titleCase(provider.Name()), digest)
	fmt.Printf("Client Secret (%s): %s\n", app, secret)
	fmt.Println()
	fmt.Print(provider.OIDCClientSnippet(auth.OIDCClient{
		Name:         app,
		ClientID:     clientID,
		SecretDigest: digest,
		RedirectURI:  "https://" + fqdn + callbackPath,
		Policy:       policy,
	}))
	return 0
}

// cmdCreateUser interactively creates a user: prompts for email (plain) and
// password (hidden, twice), hashes natively, and prints the users-database
// snippet. Print-only — users_database.yml is never written.
func cmdCreateUser(args []string) int {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		errf("Missing the <username>.")
		hint("Usage: hemma create user <username>")
		return 2
	}
	if len(args) > 1 {
		errf("Too many arguments — expected just <username>.")
		return 2
	}
	username := args[0]

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		errf("create user is interactive — run it from a terminal.")
		return 1
	}

	email, err := promptLine("Email: ")
	if err != nil {
		errf("%v", err)
		return 1
	}
	if !strings.Contains(email, "@") {
		errf("Enter a valid email address.")
		return 1
	}
	password, err := promptPassword(fd, "Password: ")
	if err != nil {
		errf("%v", err)
		return 1
	}
	if password == "" {
		errf("The password must not be empty.")
		return 1
	}
	confirm, err := promptPassword(fd, "Retype password: ")
	if err != nil {
		errf("%v", err)
		return 1
	}
	if password != confirm {
		errf("The passwords do not match.")
		return 1
	}

	provider := auth.Default()
	digest, err := provider.HashUserPassword(password)
	if err != nil {
		errf("%v", err)
		return 1
	}

	fmt.Printf("Created user %s\n\n", username)
	fmt.Print(provider.UserSnippet(username, email, digest))
	return 0
}

// titleCase upper-cases the first letter of a provider name for display
// ("authelia" -> "Authelia"), preserving authcli's output labels without the
// cli hardcoding a provider name.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// promptLine prints prompt to stderr and reads one trimmed line from stdin.
func promptLine(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// promptPassword prints prompt to stderr and reads a line with echo disabled
// (golang.org/x/term), emitting the newline the suppressed echo swallowed.
func promptPassword(fd int, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}

// usersDBWarnings runs the provider's read-only users-database cross-checks
// (auth-group typos, services nobody can access) for `doctor`. Mirrors
// oidcClientWarnings' config location logic; silent when the auth service or
// its host isn't resolvable (those misconfigurations are flagged elsewhere).
func usersDBWarnings(repoRoot string, cfg *config.Config) []string {
	if cfg.Defaults.AuthService == "" {
		return nil
	}
	authSvc, ok := cfg.Services[cfg.Defaults.AuthService]
	if !ok {
		return nil
	}
	hostM, ok := cfg.Hosts[authSvc.Host]
	if !ok {
		return nil
	}
	provider := auth.Default()
	providerCfg := filepath.Join(repoRoot, hostM.ResolvedDir(authSvc.Host), provider.ConfigPath())
	var svcs []auth.Service
	for name, s := range cfg.Services {
		if s.Auth.Mode == config.AuthNone {
			continue
		}
		svcs = append(svcs, auth.Service{Name: name, FQDN: s.FQDN, Mode: string(s.Auth.Mode), Groups: s.Auth.Groups})
	}
	return provider.ValidateUsers(providerCfg, svcs)
}
