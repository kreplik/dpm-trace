// Package integration runs an lit suite against a managed local Canton node.
//
// lit and FileCheck stay external tools we invoke; they are not ported. The
// child environment must still drop DPM_RESOLUTION_FILE and force a UTF-8
// locale, or spawned daml/damlc resolve against this component's plugin context
// instead of the target package.
package integration

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Placement puts a party on a participant. `Alice,Bob@2` means Alice on
// participant1 and Bob on participant2.
type Placement struct {
	Name  string
	Index int
}

// ParsePlacements parses the --parties argument.
// Ports parse_party_placements.
func ParsePlacements(arg string) []Placement {
	var placements []Placement
	for _, token := range strings.Split(arg, ",") {
		name, indexText, _ := strings.Cut(strings.TrimSpace(token), "@")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		index := 1
		if indexText != "" {
			// A malformed index falls back to participant 1 rather than
			// failing: the placement syntax is a convenience, not a contract.
			// int() tolerates surrounding whitespace, so "Alice@ 2" places on
			// participant 2 rather than falling back to 1.
			if parsed, err := strconv.Atoi(strings.TrimSpace(indexText)); err == nil {
				index = parsed
			}
		}
		if index < 1 {
			index = 1
		}
		placements = append(placements, Placement{Name: name, Index: index})
	}
	return placements
}

// MaxParticipant returns how many participants the placements require.
func MaxParticipant(placements []Placement) int {
	max := 1
	for _, placement := range placements {
		if placement.Index > max {
			max = placement.Index
		}
	}
	return max
}

// FreePorts reserves count ephemeral ports.
//
// All sockets are held open until every port is chosen, so the kernel cannot
// hand out the same port twice within one call. There is still a race against
// other processes between closing and Canton binding -- unavoidable without
// Canton accepting pre-bound sockets. Ports find_free_ports.
func FreePorts(count int) ([]int, error) {
	listeners := make([]net.Listener, 0, count)
	defer func() {
		for _, listener := range listeners {
			listener.Close()
		}
	}()

	ports := make([]int, 0, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

// ParticipantPorts is one participant's admin, ledger and JSON API ports.
type ParticipantPorts struct {
	Admin  int
	Ledger int
	HTTP   int
}

// ConfigText renders the Canton config. Ports canton_config_text.
func ConfigText(seqPublic, seqAdmin, medAdmin int, participants []ParticipantPorts, sequencerType string) string {
	var blocks []string
	for index, ports := range participants {
		blocks = append(blocks, fmt.Sprintf(
			"    participant%d {\n"+
				"      storage.type = memory\n"+
				"      admin-api.port = %d\n"+
				"      ledger-api.port = %d\n"+
				"      http-ledger-api.port = %d\n"+
				"    }", index+1, ports.Admin, ports.Ledger, ports.HTTP))
	}

	seqTypeLine := ""
	if sequencerType == "bft" {
		seqTypeLine = "      sequencer.type = BFT\n"
	}

	return fmt.Sprintf(`canton {
  sequencers {
    sequencer1 {
      storage.type = memory
      public-api.port = %d
      admin-api.port = %d
%s    }
  }
  mediators {
    mediator1 {
      storage.type = memory
      admin-api.port = %d
    }
  }
  participants {
%s
  }
}
`, seqPublic, seqAdmin, seqTypeLine, medAdmin, strings.Join(blocks, "\n"))
}

// BootstrapText renders the Canton bootstrap script.
// Ports canton_bootstrap_text.
func BootstrapText(dar string, placements []Placement, participants int) string {
	lines := []string{"nodes.local.start()", "bootstrap.synchronizer_local()"}
	for index := 1; index <= participants; index++ {
		lines = append(lines, fmt.Sprintf(`participant%d.synchronizers.connect_local(sequencer1, alias = "da")`, index))
	}

	active := make([]string, 0, participants)
	for index := 1; index <= participants; index++ {
		active = append(active, fmt.Sprintf(`participant%d.synchronizers.active("da")`, index))
	}
	lines = append(lines, "utils.retry_until_true { "+strings.Join(active, " && ")+" }")
	lines = append(lines, fmt.Sprintf(`participants.all.dars.upload("%s")`, dar))
	for _, placement := range placements {
		lines = append(lines, fmt.Sprintf(`participant%d.parties.enable("%s")`, placement.Index, placement.Name))
	}
	lines = append(lines, `println("=== dpm-trace integration ready ===")`)
	return strings.Join(lines, "\n") + "\n"
}

// ReadTail returns the last n lines of a file, for surfacing a Canton log when
// startup fails.
func ReadTail(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
