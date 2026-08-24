package visualizer

import (
	"fmt"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// The event tree says what happened; the state diff says what it left behind.
// Those are different questions, and the tree answers the second one badly: a
// reader wanting to know which contracts a transaction created and which it
// destroyed has to walk every event, remember that a consuming exercise is an
// archive, and pair up contract ids by eye.
//
// A transaction tree carries no archived event -- the Ledger API reports an
// archive as `consuming: true` on the exercise -- so the archived side has to
// be derived rather than read off.

// StateChange is one contract created or archived by the transaction.
type StateChange struct {
	EventID    string
	ContractID string
	Template   string
	Payload    any
}

// StateDiff splits the trace into what it created and what it archived.
func StateDiff(trace *model.Trace, order []string) (created, archived []StateChange) {
	for _, id := range order {
		ev, ok := trace.EventsByID[id]
		if !ok {
			continue
		}
		change := StateChange{
			EventID:    ev.EventID,
			ContractID: ev.ContractID,
			Template:   ev.Template,
			Payload:    ev.Payload,
		}
		switch {
		case ev.Kind == model.KindCreate:
			created = append(created, change)
		case ev.Kind == model.KindArchive:
			archived = append(archived, change)
		case ev.Kind == model.KindExercise && ev.Consuming != nil && *ev.Consuming:
			// The exercise is the archive. Its contract id is the contract
			// that ceased to exist, not one it produced.
			archived = append(archived, change)
		}
	}
	return created, archived
}

// ShowStateDiff prints the created and archived contracts.
func (s *Stepper) ShowStateDiff() {
	created, archived := StateDiff(s.Trace, s.Order)
	ctx := render.NewContext(s.Trace)

	fmt.Fprintf(s.out, "%s %s\n",
		s.Color.Apply("state diff:", "cyan"),
		fmt.Sprintf("%s created, %s archived",
			plural(len(created), "contract"), plural(len(archived), "contract")))

	if len(created) == 0 && len(archived) == 0 {
		// A reassignment-only update changes no contract on this participant.
		fmt.Fprintln(s.out, s.Color.Apply(
			"  no contract was created or archived in this projection", "gray"))
		return
	}

	s.writeChanges("+ created", created, "green", ctx)
	// The archived entries carry no payload: a consuming exercise names the
	// contract it destroyed, not its fields, and a transaction tree does not
	// contain them. So the reader sees that a contract died, not what it was.
	// Recovering it needs the contract state from somewhere else -- an ACS
	// fetch -- which is not something this artifact has.
	s.writeChanges("x archived", archived, "red", ctx)

	// The projection caveat matters more here than anywhere else: a reader
	// looking at "what this transaction did to the ledger" is exactly the
	// reader who might mistake this for the global effect. Say that in words
	// rather than repeating the header's "outside these party rights", which
	// names a concept instead of a consequence.
	fmt.Fprintln(s.out, s.Color.Apply("  "+s.projectionCaveat(), "gray"))
}

// projectionCaveat says what the reader cannot see, in terms of the parties
// they asked to read as.
func (s *Stepper) projectionCaveat() string {
	parties := s.Trace.Projection.ReadAs
	if len(parties) == 0 {
		return "the transaction may have touched other contracts this participant cannot see"
	}
	ctx := render.NewContext(s.Trace)
	names := make([]string, 0, len(parties))
	for _, party := range parties {
		names = append(names, ctx.Party(party))
	}
	return fmt.Sprintf("visible to %s only; the transaction may have touched other contracts",
		strings.Join(names, " and "))
}

func (s *Stepper) writeChanges(heading string, changes []StateChange, color string, ctx *render.Context) {
	if len(changes) == 0 {
		return
	}
	fmt.Fprintln(s.out, s.Color.Apply("  "+heading, color))
	for _, change := range changes {
		template := render.ShortTemplate(change.Template)
		if template == "" {
			template = "<unknown>"
		}
		fmt.Fprintf(s.out, "    %s %s %s\n",
			s.Color.Apply(fmt.Sprintf("%-4s", change.EventID), "gray"),
			template,
			s.Color.Apply(render.Short(change.ContractID, 40), "gray"))

		// The payload is what distinguishes two contracts of one template --
		// which of the two Split outputs is the 40 and which the 60.
		if change.Payload != nil {
			line := strings.TrimSpace(render.RenderPrettyValue(change.Payload, ctx))
			if first, _, cut := strings.Cut(line, "\n"); cut {
				line = first + " ..."
			}
			fmt.Fprintf(s.out, "      %s\n", s.Color.Apply(render.Short(line, 100), "gray"))
		}
	}
}
