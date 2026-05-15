package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	EntryQueryDefinitionVersion = "metadata.entry_query.v1"
	EntryQueryDefinitionType    = "entry_query"
	AVUQueryDefinitionType      = "avu_query"
)

// EntryQueryDefinition is the durable JSON representation of an entry query.
type EntryQueryDefinition struct {
	Version       string                 `json:"version"`
	Type          string                 `json:"type"`
	Kinds         []EntryKind            `json:"kinds,omitempty"`
	Scope         *EntryQueryScope       `json:"scope,omitempty"`
	Conditions    []EntryCondition       `json:"conditions,omitempty"`
	AVU           *AVUQuerySpec          `json:"avu,omitempty"`
	Defaults      EntryQueryDefaults     `json:"defaults,omitempty"`
	ReplicaPolicy ReplicaPolicy          `json:"replica_policy,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// AVUQuerySpec is a shorthand JSON representation for common AVU queries.
type AVUQuerySpec struct {
	Attrib string `json:"attrib,omitempty"`
	Value  string `json:"value,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

// EntryQueryDefaults stores execution defaults for a query definition.
type EntryQueryDefaults struct {
	Limit              int  `json:"limit,omitempty"`
	IncludeTotals      bool `json:"include_totals,omitempty"`
	IncludeMatchedAVUs bool `json:"include_matched_avus,omitempty"`
}

// EntryQueryExecutionOptions provides per-execution overrides for a definition.
type EntryQueryExecutionOptions struct {
	Limit              int
	Cursor             *EntryQueryCursor
	IncludeTotals      *bool
	IncludeMatchedAVUs *bool
}

// MarshalEntryQueryDefinition returns canonical entry query JSON.
func MarshalEntryQueryDefinition(definition EntryQueryDefinition) ([]byte, error) {
	canonical, err := canonicalEntryQueryDefinition(definition)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(canonical, "", "  ")
}

// ParseEntryQueryDefinition parses and validates an entry query definition.
func ParseEntryQueryDefinition(data []byte) (EntryQueryDefinition, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var definition EntryQueryDefinition
	if err := decoder.Decode(&definition); err != nil {
		return EntryQueryDefinition{}, fmt.Errorf("%w: parse entry query definition: %w", ErrInvalidEntryQuery, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return EntryQueryDefinition{}, fmt.Errorf("%w: multiple JSON values in entry query definition", ErrInvalidEntryQuery)
	}

	return canonicalEntryQueryDefinition(definition)
}

// ToEntryQuery converts a durable definition into an executable query.
func (definition EntryQueryDefinition) ToEntryQuery(options EntryQueryExecutionOptions) (EntryQuery, error) {
	canonical, err := canonicalEntryQueryDefinition(definition)
	if err != nil {
		return EntryQuery{}, err
	}

	query := EntryQuery{
		Kinds:              append([]EntryKind(nil), canonical.Kinds...),
		Scope:              canonical.Scope,
		Conditions:         append([]EntryCondition(nil), canonical.Conditions...),
		Limit:              canonical.Defaults.Limit,
		IncludeTotals:      canonical.Defaults.IncludeTotals,
		IncludeMatchedAVUs: canonical.Defaults.IncludeMatchedAVUs,
		ReplicaPolicy:      canonical.ReplicaPolicy,
		Cursor:             options.Cursor,
	}

	if options.Limit > 0 {
		query.Limit = options.Limit
	}
	if options.IncludeTotals != nil {
		query.IncludeTotals = *options.IncludeTotals
	}
	if options.IncludeMatchedAVUs != nil {
		query.IncludeMatchedAVUs = *options.IncludeMatchedAVUs
	}

	return NormalizeEntryQuery(query)
}

// EntryQueryDefinitionFromQuery creates a canonical durable definition from an
// executable query. Runtime cursor state is intentionally omitted.
func EntryQueryDefinitionFromQuery(query EntryQuery) EntryQueryDefinition {
	normalized, err := NormalizeEntryQuery(query)
	if err != nil {
		normalized = cloneEntryQuery(query)
	}

	return EntryQueryDefinition{
		Version:    EntryQueryDefinitionVersion,
		Type:       EntryQueryDefinitionType,
		Kinds:      append([]EntryKind(nil), normalized.Kinds...),
		Scope:      normalized.Scope,
		Conditions: append([]EntryCondition(nil), normalized.Conditions...),
		Defaults: EntryQueryDefaults{
			Limit:              normalized.Limit,
			IncludeTotals:      normalized.IncludeTotals,
			IncludeMatchedAVUs: normalized.IncludeMatchedAVUs,
		},
		ReplicaPolicy: normalized.ReplicaPolicy,
	}
}

func canonicalEntryQueryDefinition(definition EntryQueryDefinition) (EntryQueryDefinition, error) {
	if definition.Version == "" {
		definition.Version = EntryQueryDefinitionVersion
	}
	if definition.Version != EntryQueryDefinitionVersion {
		return EntryQueryDefinition{}, fmt.Errorf("%w: unsupported entry query definition version %q", ErrInvalidEntryQuery, definition.Version)
	}

	if definition.Type == "" {
		definition.Type = EntryQueryDefinitionType
	}
	switch definition.Type {
	case EntryQueryDefinitionType:
	case AVUQueryDefinitionType:
		if definition.AVU == nil {
			return EntryQueryDefinition{}, fmt.Errorf("%w: avu_query requires avu shorthand", ErrInvalidEntryQuery)
		}
	default:
		return EntryQueryDefinition{}, fmt.Errorf("%w: unsupported entry query definition type %q", ErrInvalidEntryQuery, definition.Type)
	}

	conditions := append([]EntryCondition(nil), definition.Conditions...)
	if definition.AVU != nil {
		conditions = append(conditions, expandAVUQuerySpec(*definition.AVU)...)
	}

	query := EntryQuery{
		Kinds:              append([]EntryKind(nil), definition.Kinds...),
		Scope:              definition.Scope,
		Conditions:         conditions,
		Limit:              definition.Defaults.Limit,
		IncludeTotals:      definition.Defaults.IncludeTotals,
		IncludeMatchedAVUs: definition.Defaults.IncludeMatchedAVUs,
		ReplicaPolicy:      definition.ReplicaPolicy,
	}
	normalized, err := NormalizeEntryQuery(query)
	if err != nil {
		return EntryQueryDefinition{}, err
	}

	canonical := EntryQueryDefinition{
		Version:    EntryQueryDefinitionVersion,
		Type:       EntryQueryDefinitionType,
		Kinds:      append([]EntryKind(nil), normalized.Kinds...),
		Scope:      normalized.Scope,
		Conditions: append([]EntryCondition(nil), normalized.Conditions...),
		Defaults: EntryQueryDefaults{
			Limit:              normalized.Limit,
			IncludeTotals:      normalized.IncludeTotals,
			IncludeMatchedAVUs: normalized.IncludeMatchedAVUs,
		},
		ReplicaPolicy: normalized.ReplicaPolicy,
		Metadata:      definition.Metadata,
	}
	return canonical, nil
}

func expandAVUQuerySpec(spec AVUQuerySpec) []EntryCondition {
	conditions := []EntryCondition{}
	for _, candidate := range []struct {
		field EntryField
		value string
	}{
		{field: FieldAVUAttrib, value: spec.Attrib},
		{field: FieldAVUValue, value: spec.Value},
		{field: FieldAVUUnit, value: spec.Unit},
	} {
		condition, ok := conditionFromPattern(candidate.field, candidate.value)
		if ok {
			conditions = append(conditions, condition)
		}
	}
	return conditions
}
