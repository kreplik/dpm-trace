package render

import (
	"regexp"
	"sort"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// Context carries the per-trace party aliases used while rendering.
// Ports RenderContext.
type Context struct {
	PartyAliases map[string]string
}

// NewContext builds the alias table for a trace.
func NewContext(trace *model.Trace) *Context {
	return &Context{PartyAliases: BuildPartyAliases(trace)}
}

// Party returns the alias for a party id, or the id unchanged.
func (c *Context) Party(value string) string {
	if c == nil {
		return value
	}
	if alias, ok := c.PartyAliases[value]; ok {
		return alias
	}
	return value
}

// PartyWithFull renders "alias (truncated-id)", or the bare id when unaliased.
func (c *Context) PartyWithFull(value string) string {
	if c == nil {
		return value
	}
	alias, ok := c.PartyAliases[value]
	if !ok || alias == "" {
		return value
	}
	return alias + " (" + ShortParty(value) + ")"
}

// RenderValue replaces party ids with aliases throughout a value, preserving
// object key order.
func (c *Context) RenderValue(value any) any {
	switch v := value.(type) {
	case string:
		return c.Party(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, c.RenderValue(item))
		}
		return out
	case *model.Object:
		out := model.NewObject()
		for _, key := range v.Keys() {
			item, _ := v.Get(key)
			out.Set(key, c.RenderValue(item))
		}
		return out
	default:
		return value
	}
}

// BuildPartyAliases maps party ids to short names. A name used by exactly one
// party becomes the bare name; a name shared by several is disambiguated with a
// fingerprint prefix. Ports build_party_aliases.
func BuildPartyAliases(trace *model.Trace) map[string]string {
	parties := map[string]bool{}
	for _, party := range trace.Projection.ReadAs {
		parties[party] = true
	}
	for _, ev := range trace.EventsByID {
		for _, group := range [][]string{ev.ActingParties, ev.Witnesses, ev.Signatories, ev.Observers} {
			for _, party := range group {
				parties[party] = true
			}
		}
		if ev.Submitter != "" {
			parties[ev.Submitter] = true
		}
		collectPartyIDs(ev.Payload, parties)
		collectPartyIDs(ev.Argument, parties)
		collectPartyIDs(ev.Result, parties)
	}

	byName := map[string][]string{}
	for party := range parties {
		name, _, ok := SplitPartyID(party)
		if !ok {
			continue
		}
		byName[name] = append(byName[name], party)
	}

	aliases := map[string]string{}
	for name, ids := range byName {
		unique := dedupeSorted(ids)
		if len(unique) == 1 {
			aliases[unique[0]] = name
			continue
		}
		for _, party := range unique {
			_, fingerprint, _ := SplitPartyID(party)
			if len(fingerprint) > 8 {
				fingerprint = fingerprint[:8]
			}
			aliases[party] = name + "@" + fingerprint
		}
	}
	return aliases
}

func collectPartyIDs(value any, parties map[string]bool) {
	switch v := value.(type) {
	case string:
		if _, _, ok := SplitPartyID(v); ok {
			parties[v] = true
		}
	case []any:
		for _, item := range v {
			collectPartyIDs(item, parties)
		}
	case *model.Object:
		for _, key := range v.Keys() {
			item, _ := v.Get(key)
			collectPartyIDs(item, parties)
		}
	}
}

var fingerprintPattern = regexp.MustCompile(`^[0-9a-fA-F]{16,}$`)

// SplitPartyID splits "Name::fingerprint", reporting whether the value looks
// like a party id at all. Ports split_party_id.
func SplitPartyID(value string) (name, fingerprint string, ok bool) {
	index := strings.Index(value, "::")
	if index < 0 {
		return "", "", false
	}
	name, fingerprint = value[:index], value[index+2:]
	if name == "" || fingerprint == "" {
		return "", "", false
	}
	if !fingerprintPattern.MatchString(fingerprint) {
		return "", "", false
	}
	return name, fingerprint, true
}

// ShortParty renders a party id as "Name::first8...last6". Ports short_party.
func ShortParty(value string) string {
	name, fingerprint, ok := SplitPartyID(value)
	if !ok {
		return Short(value, 80)
	}
	return name + "::" + fingerprint[:8] + "..." + fingerprint[len(fingerprint)-6:]
}

func dedupeSorted(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
