package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestEntryQueryBuilderExpandsAVUConvenienceConditions(t *testing.T) {
	query := NewEntryQuery().
		BothKinds().
		AVU("foo:bar", "frog*", AnyUnit).
		Limit(25).
		Build()

	normalized, err := NormalizeEntryQuery(query)
	if err != nil {
		t.Fatalf("normalize query: %v", err)
	}

	if normalized.Limit != 25 {
		t.Fatalf("expected limit 25, got %d", normalized.Limit)
	}
	if len(normalized.Kinds) != 2 {
		t.Fatalf("expected both kinds, got %+v", normalized.Kinds)
	}
	if len(normalized.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %+v", normalized.Conditions)
	}

	expected := []EntryCondition{
		{Field: FieldAVUAttrib, Op: OpEqual, Value: "foo:bar"},
		{Field: FieldAVUValue, Op: OpLike, Value: "frog*"},
	}
	for idx, condition := range expected {
		if normalized.Conditions[idx] != condition {
			t.Fatalf("condition %d mismatch: expected %+v, got %+v", idx, condition, normalized.Conditions[idx])
		}
	}
}

func TestParseEntryQueryDefinitionExpandsAVUShorthand(t *testing.T) {
	payload := []byte(`{
	  "version": "metadata.entry_query.v1",
	  "type": "avu_query",
	  "kinds": ["data_object", "collection"],
	  "scope": {"root": "/tempZone/home/alice", "mode": "descendants", "path_hintable": true},
	  "avu": {"attrib": "foo:bar", "value": "frog*", "unit": "*"},
	  "defaults": {"limit": 50, "include_matched_avus": true}
	}`)

	definition, err := ParseEntryQueryDefinition(payload)
	if err != nil {
		t.Fatalf("parse definition: %v", err)
	}

	if definition.Type != EntryQueryDefinitionType {
		t.Fatalf("expected canonical type %q, got %q", EntryQueryDefinitionType, definition.Type)
	}
	if definition.AVU != nil {
		t.Fatalf("expected canonical definition to omit avu shorthand")
	}
	if len(definition.Conditions) != 2 {
		t.Fatalf("expected expanded AVU conditions, got %+v", definition.Conditions)
	}
	if definition.Scope == nil || definition.Scope.Root != "/tempZone/home/alice" || !definition.Scope.PathHintable {
		t.Fatalf("unexpected scope %+v", definition.Scope)
	}

	query, err := definition.ToEntryQuery(EntryQueryExecutionOptions{})
	if err != nil {
		t.Fatalf("definition to query: %v", err)
	}
	if query.Limit != 50 || !query.IncludeMatchedAVUs {
		t.Fatalf("unexpected query defaults limit=%d include_matched=%t", query.Limit, query.IncludeMatchedAVUs)
	}
}

func TestMarshalEntryQueryDefinitionEmitsCanonicalConditions(t *testing.T) {
	query := NewEntryQuery().
		Collections().
		Scope("/tempZone/home/alice", EntryQueryScopeChildren).
		AVU("foo", "bar", "baz").
		Build()

	definition := EntryQueryDefinitionFromQuery(query)
	data, err := MarshalEntryQueryDefinition(definition)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}

	if bytes.Contains(data, []byte(`"avu"`)) {
		t.Fatalf("expected canonical JSON to omit avu shorthand: %s", string(data))
	}

	var decoded EntryQueryDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal canonical JSON: %v", err)
	}
	if decoded.Type != EntryQueryDefinitionType {
		t.Fatalf("expected type %q, got %q", EntryQueryDefinitionType, decoded.Type)
	}
	if len(decoded.Conditions) != 3 {
		t.Fatalf("expected 3 canonical conditions, got %+v", decoded.Conditions)
	}
}

func TestParseEntryQueryDefinitionRejectsUnsupportedBooleanGrouping(t *testing.T) {
	payload := []byte(`{
	  "version": "metadata.entry_query.v1",
	  "type": "entry_query",
	  "or": [
	    {"conditions": [{"field": "avu.attrib", "op": "=", "value": "foo"}]}
	  ]
	}`)

	_, err := ParseEntryQueryDefinition(payload)
	if !errors.Is(err, ErrInvalidEntryQuery) {
		t.Fatalf("expected ErrInvalidEntryQuery, got %v", err)
	}
}

func TestNormalizeEntryQueryRejectsInvalidCursorOffset(t *testing.T) {
	query := NewEntryQuery().
		BothKinds().
		Cursor(&EntryQueryCursor{
			Collections: EntryBranchCursor{Offset: -1},
		}).
		Build()

	_, err := NormalizeEntryQuery(query)
	if !errors.Is(err, ErrInvalidEntryQuery) {
		t.Fatalf("expected ErrInvalidEntryQuery, got %v", err)
	}
}

func TestNormalizeEntryQueryRejectsInclusiveScopeModes(t *testing.T) {
	modes := []EntryQueryScopeMode{
		EntryQueryScopeSelfAndChildren,
		EntryQueryScopeSelfAndDescendants,
	}
	for _, mode := range modes {
		query := NewEntryQuery().
			Collections().
			Scope("/tempZone/home/alice", mode).
			AVUAttrib("project").
			Build()

		_, err := NormalizeEntryQuery(query)
		if !errors.Is(err, ErrInvalidEntryQuery) {
			t.Fatalf("expected ErrInvalidEntryQuery for mode %q, got %v", mode, err)
		}
	}
}
