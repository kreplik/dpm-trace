package integration

import (
	"strings"
	"testing"
)

func TestParsePlacements(t *testing.T) {
	got := ParsePlacements("Alice,Bob@2, Carol@3 ")
	want := []Placement{{"Alice", 1}, {"Bob", 2}, {"Carol", 3}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if n := MaxParticipant(got); n != 3 {
		t.Errorf("MaxParticipant = %d, want 3", n)
	}
}

// A malformed or absent index falls back to participant 1 rather than failing:
// the placement syntax is a convenience, not a contract.
func TestParsePlacementsToleratesBadIndex(t *testing.T) {
	for _, arg := range []string{"Alice@x", "Alice@", "Alice@0", "Alice@-2"} {
		got := ParsePlacements(arg)
		if len(got) != 1 || got[0].Index != 1 {
			t.Errorf("%q -> %v, want index 1", arg, got)
		}
	}
	if got := ParsePlacements(",,"); len(got) != 0 {
		t.Errorf("empty names should be dropped, got %v", got)
	}
}

func TestFreePortsAreDistinct(t *testing.T) {
	ports, err := FreePorts(9)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 9 {
		t.Fatalf("got %d ports, want 9", len(ports))
	}
	seen := map[int]bool{}
	for _, port := range ports {
		if seen[port] {
			t.Errorf("port %d handed out twice", port)
		}
		seen[port] = true
	}
}

func TestConfigTextRendersEveryParticipant(t *testing.T) {
	config := ConfigText(100, 101, 102, []ParticipantPorts{{200, 201, 202}, {300, 301, 302}}, "bft")
	for _, want := range []string{
		"public-api.port = 100", "admin-api.port = 101", "admin-api.port = 102",
		"participant1 {", "participant2 {",
		"http-ledger-api.port = 202", "http-ledger-api.port = 302",
		"sequencer.type = BFT",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("config missing %q:\n%s", want, config)
		}
	}
	if plain := ConfigText(100, 101, 102, []ParticipantPorts{{200, 201, 202}}, "reference"); strings.Contains(plain, "BFT") {
		t.Error("non-bft sequencer should not declare a BFT type")
	}
}

func TestBootstrapTextConnectsAndPlacesParties(t *testing.T) {
	script := BootstrapText("/tmp/app.dar", []Placement{{"Alice", 1}, {"Bob", 2}}, 2)
	for _, want := range []string{
		"nodes.local.start()",
		`participant1.synchronizers.connect_local(sequencer1, alias = "da")`,
		`participant2.synchronizers.connect_local(sequencer1, alias = "da")`,
		`participants.all.dars.upload("/tmp/app.dar")`,
		`participant1.parties.enable("Alice")`,
		`participant2.parties.enable("Bob")`,
		"=== dpm-trace integration ready ===",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("bootstrap missing %q:\n%s", want, script)
		}
	}
	// Every participant must be active before the DAR upload, or it races.
	if strings.Index(script, "retry_until_true") > strings.Index(script, "dars.upload") {
		t.Error("the readiness wait must precede the DAR upload")
	}
}
