package cli

import (
	"os"
	"testing"
)

// TestMain stubs doctor's known_hosts lookup for the WHOLE package: fixture
// hosts don't exist, so the real ssh-keygen -F would report every one of them
// unknown and flip doctor exit codes across unrelated tests. Individual tests
// override and restore the hook themselves.
func TestMain(m *testing.M) {
	hostKnown = func(string) bool { return true }
	os.Exit(m.Run())
}

// stubKnown swaps the known_hosts hook for one test and restores it after.
func stubKnown(t *testing.T, known func(string) bool) {
	t.Helper()
	old := hostKnown
	hostKnown = known
	t.Cleanup(func() { hostKnown = old })
}

// A host whose ip AND ssh-dest are both absent from known_hosts is a problem.
// In tests localHost matches nothing, so BOTH role-carrying hosts are remote.
func TestCheckDeployReadiness_UnknownHost(t *testing.T) {
	stubKnown(t, func(string) bool { return false })
	if got := checkDeployReadiness(deployCfg()); got != 2 {
		t.Errorf("two unknown hosts should count 2 problems, got %d", got)
	}
}

// The check passes if EITHER the ip or the ssh-dest host part is known
// (resolver is pinned known so only appbox's lookups vary).
func TestCheckDeployReadiness_KnownByEitherName(t *testing.T) {
	for _, knownName := range []string{"192.0.2.2", "appbox.lan"} {
		asked := []string{}
		stubKnown(t, func(h string) bool {
			asked = append(asked, h)
			return h == knownName || h == "resolver" || h == "192.0.2.1"
		})
		if got := checkDeployReadiness(deployCfg()); got != 0 {
			t.Errorf("host known as %q should pass, got %d problems (lookups: %v)", knownName, got, asked)
		}
	}
}

// Only remote deploy targets are checked: role-less hosts (spare) never reach
// the lookup, and an empty name is never known.
func TestCheckDeployReadiness_ChecksOnlyDeployTargets(t *testing.T) {
	asked := map[string]bool{}
	stubKnown(t, func(h string) bool { asked[h] = true; return true })
	if got := checkDeployReadiness(deployCfg()); got != 0 {
		t.Errorf("all known should pass, got %d problems", got)
	}
	if asked["192.0.2.9"] || asked["spare"] {
		t.Errorf("role-less host spare must not be checked, lookups: %v", asked)
	}
}

// The @-split for the known_hosts lookup: user@host → host, alias unchanged.
func TestSSHHostPart(t *testing.T) {
	cases := map[string]string{
		"guldmund@10.0.30.200": "10.0.30.200",
		"optiplex":             "optiplex",
		"a@b@c":                "c",
	}
	for in, want := range cases {
		if got := sshHostPart(in); got != want {
			t.Errorf("sshHostPart(%q) = %q, want %q", in, got, want)
		}
	}
}
