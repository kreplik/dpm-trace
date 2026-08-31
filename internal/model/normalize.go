package model

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

// variantKind pairs a Ledger API event-variant wrapper with the kind it means.
type variantKind struct {
	key  string
	kind string
}

// eventVariantKinds ports EVENT_VARIANT_KINDS.
//
// The reassignment entries come first for a reason: an assigned event nests a
// created event, so matching "assigned" before "createdEvent" keeps it labeled
// as a reassignment. The Js* names are what the JSON API actually emits,
// verified against Canton 3.5 (tests/fixtures/reassignment/real-*.json); the
// others are tolerance for other shapes and are not independently confirmed.
var eventVariantKinds = []variantKind{
	{"JsUnassignedEvent", KindUnassign},
	{"JsAssignmentEvent", KindAssign},
	{"unassigned", KindUnassign},
	{"UnassignedEvent", KindUnassign},
	{"unassignedEvent", KindUnassign},
	{"assigned", KindAssign},
	{"AssignedEvent", KindAssign},
	{"assignedEvent", KindAssign},
	{"created", KindCreate},
	{"CreatedEvent", KindCreate},
	{"createdEvent", KindCreate},
	{"exercised", KindExercise},
	{"ExercisedEvent", KindExercise},
	{"exercisedEvent", KindExercise},
	{"archived", KindArchive},
	{"ArchivedEvent", KindArchive},
	{"archivedEvent", KindArchive},
}

// NormalizeTrace turns a raw update response into a Trace. Ports normalize_trace.
func NormalizeTrace(raw *Object, source, sourceURL string, parties []string) (*Trace, error) {
	tx := unwrapTransaction(raw)

	updateID := pickString(tx, "update_id", "updateId", "id")
	if updateID == "" {
		updateID = pickString(raw, "update_id", "updateId")
	}
	if updateID == "" {
		return nil, errors.New("could not find update_id/updateId in response")
	}

	eventsRaw := pick(tx, "events_by_id", "eventsById", "events")
	if eventsRaw == nil {
		// Single-event reassignment shape: {"reassignment": {"event": {...}}}.
		if single, ok := pickObject(tx, "event"); ok {
			eventsRaw = []any{single}
		}
	}
	eventsByID, eventOrder := normalizeEventsMap(eventsRaw)

	rootIDs := listString(pick(tx, "root_event_ids", "rootEventIds"))
	if len(rootIDs) == 0 {
		rootIDs = inferRoots(eventsByID, eventOrder)
	}

	note := participantNote
	if source == "scan" {
		note = scanNote
	}

	trace := &Trace{
		UpdateID:  updateID,
		Source:    source,
		SourceURL: sourceURL,
		Projection: Projection{
			Source:           source,
			ParticipantScope: source != "scan",
			ReadAs:           parties,
			NotGlobal:        source != "scan",
			Note:             note,
		},
		RootEventIDs:   rootIDs,
		EventsByID:     eventsByID,
		RecordTime:     pickString(tx, "record_time", "recordTime"),
		Offset:         pickString(tx, "offset"),
		SynchronizerID: pickString(tx, "synchronizer_id", "synchronizerId"),
		Raw:            raw,
	}
	if err := trace.CheckAcyclic(); err != nil {
		return nil, err
	}
	return trace, nil
}

// unwrapTransaction descends through the envelopes the API and artifacts wrap
// updates in, until it reaches something holding events. Ports unwrap_transaction.
func unwrapTransaction(raw *Object) *Object {
	data := raw
	if inner, ok := pickObject(raw, "data"); ok {
		data = inner
	}
	for _, key := range []string{"transaction", "Transaction", "TransactionTree", "reassignment", "Reassignment", "update", "Update"} {
		if inner, ok := pickObject(data, key); ok {
			return unwrapTransaction(inner)
		}
	}
	if inner, ok := pickObject(data, "value"); ok {
		return unwrapTransaction(inner)
	}
	return data
}

// normalizeEventsMap accepts either the map or list encoding of an event
// collection. Ports normalize_events_map.
func normalizeEventsMap(eventsRaw any) (map[string]*Event, []string) {
	result := make(map[string]*Event)
	var order []string

	add := func(id string, value any) {
		obj, ok := asObject(value)
		if !ok {
			return
		}
		ev := NormalizeEvent(id, obj)
		if _, seen := result[ev.EventID]; !seen {
			order = append(order, ev.EventID)
		}
		result[ev.EventID] = ev
	}

	switch events := eventsRaw.(type) {
	case *Object:
		// Document order, the same order Python's dict iteration yields.
		for _, key := range events.Keys() {
			value, _ := events.Get(key)
			add(key, value)
		}
	case []any:
		for i, item := range events {
			if entry, ok := asObject(item); ok {
				key, hasKey := entry.Get("key")
				value, hasValue := entry.Get("value")
				if hasKey && hasValue {
					add(toString(key), value)
					continue
				}
			}
			add(strconv.Itoa(i), item)
		}
	}

	linkRangeChildren(result)
	return result, order
}

// NormalizeEvent turns one raw event into an Event. Ports normalize_event.
func NormalizeEvent(eventID string, raw *Object) *Event {
	variant := raw
	kind := KindEvent
	for _, vk := range eventVariantKinds {
		if inner, ok := pickObject(raw, vk.key); ok {
			variant = inner
			kind = vk.kind
			break
		}
	}
	if kind == KindEvent {
		kind = kindFromExplicit(pickString(raw, "eventType", "event_type", "kind"))
	}

	// Some JSON API variants wrap their payload in a {"value": ...} envelope --
	// JsUnassignedEvent does, JsAssignmentEvent does not.
	if kind != KindEvent {
		if inner, ok := pickObject(variant, "value"); ok {
			variant = inner
		}
	}

	// An assigned event carries the reassigned contract in a nested created
	// event; look there for the contract data and in the event itself for the
	// reassignment metadata.
	sources := []*Object{variant}
	if nested, ok := pickObject(variant, "created_event", "createdEvent"); ok {
		sources = append(sources, nested)
	}
	field := func(keys ...string) any {
		for _, src := range sources {
			if value := pick(src, keys...); value != nil {
				return value
			}
		}
		return nil
	}

	// nodeId 0 is a real id, so fall back on absence rather than emptiness --
	// reassignment events are commonly the only node in their update.
	resolvedID := eventID
	if id := field("event_id", "eventId", "node_id", "nodeId"); id != nil {
		if s := toString(id); s != "" {
			resolvedID = s
		}
	}

	return &Event{
		EventID:             resolvedID,
		Kind:                kind,
		Template:            templateName(field("template_id", "templateId")),
		ContractID:          toString(field("contract_id", "contractId")),
		Choice:              toString(field("choice")),
		Consuming:           asBool(field("consuming")),
		ActingParties:       listString(field("acting_parties", "actingParties")),
		Witnesses:           listString(field("witness_parties", "witnessParties", "witnesses")),
		Signatories:         listString(field("signatories")),
		Observers:           listString(field("observers")),
		ChildEventIDs:       listString(field("child_event_ids", "childEventIds")),
		Payload:             field("create_arguments", "createArguments", "create_argument", "createArgument", "payload"),
		Argument:            field("choice_argument", "choiceArgument", "exercise_argument", "exerciseArgument", "argument"),
		Result:              field("exercise_result", "exerciseResult", "result"),
		SourceSynchronizer:  pickString(variant, "source", "source_synchronizer", "sourceSynchronizer"),
		TargetSynchronizer:  pickString(variant, "target", "target_synchronizer", "targetSynchronizer"),
		ReassignmentID:      pickString(variant, "reassignment_id", "reassignmentId", "unassign_id", "unassignId"),
		ReassignmentCounter: asInt(pick(variant, "reassignment_counter", "reassignmentCounter")),
		Submitter:           pickString(variant, "submitter"),
		Raw:                 raw,
	}
}

func kindFromExplicit(explicit string) string {
	lower := strings.ToLower(explicit)
	switch {
	case strings.Contains(lower, "create"):
		return KindCreate
	case strings.Contains(lower, "exercise"):
		return KindExercise
	case strings.Contains(lower, "archive"):
		return KindArchive
	case strings.Contains(lower, "unassign"):
		return KindUnassign
	case strings.Contains(lower, "assign"):
		return KindAssign
	}
	return KindEvent
}

// inferRoots returns the events that are nobody's child, in document order.
// Ports infer_roots.
//
// Order is load-bearing rather than cosmetic: both compare paths pair root
// events positionally, so a different order produces spurious only-in-A /
// only-in-B rows. Python iterates the events dict, which preserves insertion
// order, so the id order recorded during normalization is carried here instead
// of sorting -- a lexical sort puts "#10:0" before "#1:0".
func inferRoots(eventsByID map[string]*Event, order []string) []string {
	children := make(map[string]bool)
	for _, ev := range eventsByID {
		for _, child := range ev.ChildEventIDs {
			children[child] = true
		}
	}
	var roots []string
	for _, id := range order {
		if !children[id] {
			roots = append(roots, id)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, order...)
	}
	return roots
}

// linkRangeChildren reconstructs parent/child links for the numeric node-id
// encoding, where a node covers every id up to lastDescendantNodeId.
// Ports link_range_children.
func linkRangeChildren(eventsByID map[string]*Event) {
	var numeric []string
	for id := range eventsByID {
		if _, err := strconv.Atoi(id); err == nil {
			numeric = append(numeric, id)
		}
	}
	if len(numeric) == 0 {
		return
	}
	sortEventIDs(numeric)

	lastDescendant := func(id string) (int, bool) {
		ev := eventsByID[id]
		variant := eventVariant(ev.Raw)
		value := pick(variant, "last_descendant_node_id", "lastDescendantNodeId")
		if n := asInt(value); n != nil {
			return int(*n), true
		}
		return 0, false
	}

	for position, id := range numeric {
		ev := eventsByID[id]
		if len(ev.ChildEventIDs) > 0 {
			continue
		}
		last, ok := lastDescendant(id)
		if !ok {
			continue
		}

		// Walk forward taking direct children only: after claiming one, skip
		// past its own descendant range, because those belong to it rather
		// than to us. Claiming every id in (self, last] instead would attach
		// grandchildren to the root as well, so a nested event renders twice
		// and --export persists the duplicate.
		var children []string
		for index := position + 1; index < len(numeric); index++ {
			childID := numeric[index]
			childNode, err := strconv.Atoi(childID)
			if err != nil || childNode > last {
				break
			}
			children = append(children, childID)

			childLast, ok := lastDescendant(childID)
			if !ok || childLast <= childNode {
				continue
			}
			for index+1 < len(numeric) {
				next, err := strconv.Atoi(numeric[index+1])
				if err != nil || next > childLast {
					break
				}
				index++
			}
		}
		ev.ChildEventIDs = children
	}
}

// eventVariant returns the wrapped payload of a raw event, or the event itself.
// Ports event_variant.
func eventVariant(raw *Object) *Object {
	for _, vk := range eventVariantKinds {
		if inner, ok := pickObject(raw, vk.key); ok {
			if value, ok := pickObject(inner, "value"); ok {
				return value
			}
			return inner
		}
	}
	return raw
}

// SortEventIDs orders numerically when every id is numeric, lexically
// otherwise, so output does not depend on map iteration order.
func SortEventIDs(ids []string) { sortEventIDs(ids) }

func sortEventIDs(ids []string) {
	allNumeric := true
	for _, id := range ids {
		if _, err := strconv.Atoi(id); err != nil {
			allNumeric = false
			break
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if allNumeric {
			a, _ := strconv.Atoi(ids[i])
			b, _ := strconv.Atoi(ids[j])
			return a < b
		}
		return ids[i] < ids[j]
	})
}
