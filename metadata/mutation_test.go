package metadata

import (
	"errors"
	"testing"
)

func TestMutationServiceReplacePathAVUNormalizesAndDelegates(t *testing.T) {
	mutator := &testAVUMutator{}
	service, err := NewMutationService(mutator)
	if err != nil {
		t.Fatalf("new mutation service: %v", err)
	}

	updated, err := service.ReplacePathAVU(" /tempZone/home/test1/object.txt/ ", AVUReplacement{
		From: AVUStat{Name: " source ", Value: " before ", Units: " unit "},
		To:   AVUStat{Name: " source ", Value: " after ", Units: " unit "},
	})
	if err != nil {
		t.Fatalf("replace path AVU: %v", err)
	}

	if mutator.path != "/tempZone/home/test1/object.txt" {
		t.Fatalf("expected normalized path, got %q", mutator.path)
	}
	if mutator.replacement.From.Name != "source" || mutator.replacement.From.Value != " before " || mutator.replacement.From.Units != " unit " {
		t.Fatalf("expected normalized from AVU name, got %+v", mutator.replacement.From)
	}
	if mutator.replacement.To.Name != "source" || mutator.replacement.To.Value != " after " || mutator.replacement.To.Units != " unit " {
		t.Fatalf("expected normalized to AVU name, got %+v", mutator.replacement.To)
	}
	if updated.Value != " after " {
		t.Fatalf("expected returned replacement, got %+v", updated)
	}
}

func TestMutationServiceReplacePathAVURejectsInvalidPath(t *testing.T) {
	service, err := NewMutationService(&testAVUMutator{})
	if err != nil {
		t.Fatalf("new mutation service: %v", err)
	}

	_, err = service.ReplacePathAVU("relative/path", AVUReplacement{
		From: AVUStat{Name: "source", Value: "before"},
		To:   AVUStat{Name: "source", Value: "after"},
	})
	if !errors.Is(err, ErrInvalidIRODSPath) {
		t.Fatalf("expected ErrInvalidIRODSPath, got %v", err)
	}
}

func TestNormalizeAVUReplacementRequiresFromAndToNameValue(t *testing.T) {
	cases := []struct {
		name        string
		replacement AVUReplacement
	}{
		{name: "missing from name", replacement: AVUReplacement{From: AVUStat{Value: "before"}, To: AVUStat{Name: "source", Value: "after"}}},
		{name: "missing from value", replacement: AVUReplacement{From: AVUStat{Name: "source"}, To: AVUStat{Name: "source", Value: "after"}}},
		{name: "missing to name", replacement: AVUReplacement{From: AVUStat{Name: "source", Value: "before"}, To: AVUStat{Value: "after"}}},
		{name: "missing to value", replacement: AVUReplacement{From: AVUStat{Name: "source", Value: "before"}, To: AVUStat{Name: "source"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeAVUReplacement(tc.replacement)
			if !errors.Is(err, ErrInvalidAVUReplacement) {
				t.Fatalf("expected ErrInvalidAVUReplacement, got %v", err)
			}
		})
	}
}

func TestNewMutationServiceRequiresMutator(t *testing.T) {
	_, err := NewMutationService(nil)
	if !errors.Is(err, ErrMissingAVUMutator) {
		t.Fatalf("expected ErrMissingAVUMutator, got %v", err)
	}
}

type testAVUMutator struct {
	path        string
	replacement AVUReplacement
}

func (mutator *testAVUMutator) ReplacePathAVU(irodsPath string, replacement AVUReplacement) (AVUStat, error) {
	mutator.path = irodsPath
	mutator.replacement = replacement
	return replacement.To, nil
}
