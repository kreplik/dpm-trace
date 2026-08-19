package visualizer

import (
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

func event(id, kind, template, choice, contractID string, parties ...string) *model.Event {
	return &model.Event{
		EventID: id, Kind: kind, Template: template, Choice: choice,
		ContractID: contractID, ActingParties: parties,
	}
}

func TestParseFilterQualifiedAndBare(t *testing.T) {
	for _, field := range filterFields {
		got, err := ParseFilter(field + " value")
		if err != nil {
			t.Fatalf("%s: %v", field, err)
		}
		if got.Field != field || got.Value != "value" {
			t.Errorf("%s parsed as %+v", field, got)
		}
	}

	// A word that is not a field name is a bare search, not an error: a reader
	// typing a contract id fragment should not have to name the field.
	got, err := ParseFilter("Asset:Asset")
	if err != nil {
		t.Fatal(err)
	}
	if got.Field != "" || got.Value != "Asset:Asset" {
		t.Errorf("bare search parsed as %+v", got)
	}

	// A multi-word bare search stays whole.
	if got, _ := ParseFilter("some phrase"); got.Value != "some phrase" {
		t.Errorf("phrase parsed as %+v", got)
	}
}

func TestParseFilterRejectsEmptyAndMissingValue(t *testing.T) {
	if _, err := ParseFilter(""); err == nil {
		t.Error("empty filter accepted")
	}
	for _, arg := range []string{"choice", "party ", "template"} {
		if _, err := ParseFilter(arg); err == nil {
			t.Errorf("%q accepted with no value", arg)
		}
	}
}

func TestFilterMatchesEachField(t *testing.T) {
	ev := event("0", "exercise", "pkg123:Asset:Asset", "Split", "00abcdef",
		"Alice::1220aa")
	ev.Witnesses = []string{"Issuer::1220bb"}

	for _, tc := range []struct {
		filter Filter
		want   bool
		why    string
	}{
		{Filter{"template", "Asset"}, true, "short template"},
		{Filter{"template", "pkg123"}, true, "full template includes the package"},
		{Filter{"choice", "split"}, true, "case-insensitive"},
		{Filter{"choice", "Burn"}, false, "different choice"},
		{Filter{"party", "Alice"}, true, "acting party"},
		{Filter{"party", "Issuer"}, true, "witness counts as a party"},
		{Filter{"party", "Mallory"}, false, "absent party"},
		{Filter{"contract", "abcdef"}, true, "contract id fragment"},
		{Filter{"kind", "exercise"}, true, "event kind"},
		{Filter{"kind", "create"}, false, "wrong kind"},
		{Filter{"package", "pkg123"}, true, "package id"},
		{Filter{"id", "0"}, true, "event id"},
	} {
		if got := tc.filter.Matches(ev); got != tc.want {
			t.Errorf("%+v = %v, want %v (%s)", tc.filter, got, tc.want, tc.why)
		}
	}
}

// The unqualified form searches every field, which is what a reader reaching
// for a half-remembered string expects.
func TestFilterUnqualifiedSearchesEverything(t *testing.T) {
	ev := event("2", "create", "pkg123:Asset:Asset", "", "00abcdef", "Alice::1220aa")
	for _, value := range []string{"Asset", "abcdef", "create", "Alice", "pkg123", "2"} {
		if !(Filter{Value: value}).Matches(ev) {
			t.Errorf("bare search for %q did not match", value)
		}
	}
	if (Filter{Value: "nothing-here"}).Matches(ev) {
		t.Error("bare search matched an absent value")
	}
}

// A value a reader saw on screen -- an asset name, a quantity -- must be
// findable, or `find GOLD` reports nothing for a contract plainly named GOLD.
func TestFilterMatchesRenderedValues(t *testing.T) {
	payload := model.NewObject()
	payload.Set("name", "GOLD")
	payload.Set("quantity", "100")
	ev := event("1", "create", "pkg:Asset:Asset", "", "00abc")
	ev.Payload = payload

	for _, value := range []string{"GOLD", "gold", "quantity"} {
		if !(Filter{Value: value}).Matches(ev) {
			t.Errorf("bare search for %q did not reach the payload", value)
		}
	}
	if !(Filter{"payload", "GOLD"}).Matches(ev) {
		t.Error("payload filter did not match")
	}
	// The payload field must not match metadata, or it is just another bare search.
	if (Filter{"payload", "Asset:Asset"}).Matches(ev) {
		t.Error("payload filter matched the template")
	}
}

func TestFilterEmptyValueMatchesEverything(t *testing.T) {
	if !(Filter{}).Matches(event("0", "create", "", "", "")) {
		t.Error("an empty filter should not exclude anything")
	}
}

func TestFilterDescribe(t *testing.T) {
	if got := (Filter{"party", "Alice"}).Describe(); got != "party Alice" {
		t.Errorf("got %q", got)
	}
	if got := (Filter{Value: "Alice"}).Describe(); got != "Alice" {
		t.Errorf("got %q", got)
	}
}

// Navigation must skip non-matching steps, and stop rather than wrap.
func TestFilteredNavigation(t *testing.T) {
	s, buf := newStepper(t)
	if len(s.Order) < 2 {
		t.Skip("fixture too small")
	}

	s.Dispatch("filter kind create")
	buf.Reset()
	s.Dispatch("n")

	current := s.Trace.EventsByID[s.Order[s.Index]]
	if current.Kind != model.KindCreate {
		// Either it moved to a create, or it reported that none remained.
		if !strings.Contains(buf.String(), "no further match") {
			t.Errorf("landed on %q with no explanation:\n%s", current.Kind, buf.String())
		}
	}
}

// A filter selecting nothing must not be applied: navigation would be stranded
// with every step excluded and no way to see why.
func TestFilterSelectingNothingIsNotApplied(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("filter template DefinitelyNotPresent")

	if s.Active != nil {
		t.Error("an empty filter result was applied")
	}
	if !strings.Contains(buf.String(), "no events match") {
		t.Errorf("no explanation given:\n%s", buf.String())
	}
}

func TestFilterClearAndMatches(t *testing.T) {
	s, buf := newStepper(t)

	s.Dispatch("matches")
	if !strings.Contains(buf.String(), "no filter set") {
		t.Errorf("matches without a filter = %q", buf.String())
	}

	buf.Reset()
	s.Dispatch("filter kind exercise")
	if s.Active == nil {
		t.Fatalf("filter not applied:\n%s", buf.String())
	}

	buf.Reset()
	s.Dispatch("matches")
	if !strings.Contains(buf.String(), "matches:") {
		t.Errorf("matches = %q", buf.String())
	}

	buf.Reset()
	s.Dispatch("filter")
	if s.Active != nil {
		t.Error("filter not cleared")
	}
	if !strings.Contains(buf.String(), "filter cleared") {
		t.Errorf("clear = %q", buf.String())
	}
}

// `help` must name every field the parser accepts. A hand-written list drifted
// from filterFields once already, leaving `payload` undocumented.
func TestHelpNamesEveryFilterField(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("help")

	out := buf.String()
	for _, field := range filterFields {
		if !strings.Contains(out, field) {
			t.Errorf("help does not mention the %q field:\n%s", field, out)
		}
	}
}

// The startup banner is written separately from `help`, so it drifts the same
// way. A reader who never types `help` learns the commands only from here.
func TestBannerMentionsSearch(t *testing.T) {
	s, buf := newStepper(t)
	s.Run(strings.NewReader("q\n"))

	for _, cmd := range []string{"filter", "find", "matches"} {
		if !strings.Contains(buf.String(), cmd) {
			t.Errorf("banner does not mention %q:\n%s", cmd, buf.String())
		}
	}
}

// A typo must not be swallowed as a bare search: `filterfoo` is a mistake, and
// silently searching for "foo" hides it.
func TestFilterPrefixDoesNotSwallowTypos(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("filterfoo")
	if !strings.Contains(buf.String(), "unknown command") {
		t.Errorf("`filterfoo` = %q, want it rejected", buf.String())
	}
}

// find moves without changing what navigation is restricted to.
func TestFindDoesNotSetTheFilter(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("find Token")
	if s.Active != nil {
		t.Error("find set the active filter")
	}
	if buf.Len() == 0 {
		t.Error("find produced no output")
	}
}
