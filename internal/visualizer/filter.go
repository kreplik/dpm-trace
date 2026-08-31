package visualizer

import (
	"fmt"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// A Filter narrows the stepper to the events a reader cares about. A large
// transaction is mostly noise for any given question -- "what did Alice sign",
// "where did this contract go" -- and scrolling a hundred steps to find three
// is what the filter replaces.
//
// Fields are matched case-insensitively on a substring, because the values a
// reader has to hand are partial: the tail of a contract id copied from a log,
// a party name without its fingerprint, a module without its package id.
type Filter struct {
	Field string // one of filterFields, or "" to match any of them
	Value string

	// Ctx renders payloads the way the screen does, so a needle copied off it
	// matches. Without it the screen shows the party alias while the filter
	// searches the full id, and a needle spanning past an alias misses.
	Ctx *render.Context
}

// filterFields are the fields a filter may name. The first six are the
// milestone's requirement; kind and package are here because they are the two
// questions the others cannot express ("show me the archives", "show me
// everything from this package"), and synchronizer because a reassignment's
// two ends are not addressable by any of the others.
var filterFields = []string{
	"template", "choice", "party", "contract", "kind", "package", "id", "payload",
	"synchronizer",
}

// ParseFilter reads `filter <field> <value>` or `filter <value>`.
//
// The bare form searches every field, which is what a reader reaching for a
// half-remembered string wants; the qualified form is for when that returns
// too much.
func ParseFilter(arg string) (Filter, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Filter{}, fmt.Errorf("usage: filter [%s] <value>", strings.Join(filterFields, "|"))
	}

	field, value, hasField := strings.Cut(arg, " ")
	lowered := strings.ToLower(field)
	for _, known := range filterFields {
		if lowered != known {
			continue
		}
		value = strings.TrimSpace(value)
		if !hasField || value == "" {
			return Filter{}, fmt.Errorf("filter %s needs a value", known)
		}
		return Filter{Field: known, Value: value}, nil
	}
	return Filter{Value: arg}, nil
}

// Matches reports whether an event satisfies the filter.
func (f Filter) Matches(ev *model.Event) bool {
	if f.Value == "" {
		return true
	}
	needle := strings.ToLower(f.Value)

	switch f.Field {
	case "template":
		return containsFold(ev.Template, needle) || containsFold(render.ShortTemplate(ev.Template), needle)
	case "choice":
		return containsFold(ev.Choice, needle)
	case "party":
		// Submitter is the acting party of a reassignment, which carries none
		// of the other four lists.
		return anyContainsFold(needle, ev.ActingParties, ev.Witnesses, ev.Signatories, ev.Observers) ||
			containsFold(ev.Submitter, needle)
	case "contract":
		return containsFold(ev.ContractID, needle)
	case "kind":
		return containsFold(ev.Kind, needle)
	case "package":
		return containsFold(render.PackageFromTemplate(ev.Template), needle)
	case "id":
		return containsFold(ev.EventID, needle) || containsFold(ev.ReassignmentID, needle)
	case "synchronizer":
		// Either end matches: a reader asking where a contract went knows one
		// of the two synchronizers, rarely which end of the move they have.
		return containsFold(ev.SourceSynchronizer, needle) ||
			containsFold(ev.TargetSynchronizer, needle)
	case "payload":
		return f.matchesValues(needle, ev)
	}

	// Unqualified: any of the above.
	return containsFold(ev.Template, needle) ||
		containsFold(render.ShortTemplate(ev.Template), needle) ||
		containsFold(ev.Choice, needle) ||
		containsFold(ev.ContractID, needle) ||
		containsFold(ev.Kind, needle) ||
		containsFold(ev.EventID, needle) ||
		containsFold(render.PackageFromTemplate(ev.Template), needle) ||
		anyContainsFold(needle, ev.ActingParties, ev.Witnesses, ev.Signatories, ev.Observers) ||
		// Reassignment metadata: rendering makes these first-class, so search
		// has to as well, or the only searchable thing about an unassign is its
		// template.
		containsFold(ev.Submitter, needle) ||
		containsFold(ev.ReassignmentID, needle) ||
		containsFold(ev.SourceSynchronizer, needle) ||
		containsFold(ev.TargetSynchronizer, needle) ||
		// Payload last: a reader searching for a value they saw on screen --
		// an asset name, a quantity -- expects it to be found, and metadata
		// matches are the cheaper ones to try first.
		f.matchesValues(needle, ev)
}

// matchesValues searches the rendered payload, choice argument and result. The
// rendered form is what the reader saw, so what they type matches what is on
// screen rather than the wire encoding.
func (f Filter) matchesValues(loweredNeedle string, ev *model.Event) bool {
	for _, value := range []any{ev.Payload, ev.Argument, ev.Result} {
		if value == nil {
			continue
		}
		if containsFold(render.RenderPrettyValue(value, f.Ctx), loweredNeedle) {
			return true
		}
	}
	return false
}

// Describe renders the filter for the prompt and the status line.
func (f Filter) Describe() string {
	if f.Field == "" {
		return f.Value
	}
	return f.Field + " " + f.Value
}

func containsFold(haystack, loweredNeedle string) bool {
	return haystack != "" && strings.Contains(strings.ToLower(haystack), loweredNeedle)
}

func anyContainsFold(loweredNeedle string, lists ...[]string) bool {
	for _, list := range lists {
		for _, value := range list {
			if containsFold(value, loweredNeedle) {
				return true
			}
		}
	}
	return false
}
