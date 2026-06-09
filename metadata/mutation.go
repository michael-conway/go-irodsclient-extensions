package metadata

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidAVUReplacement = errors.New("invalid avu replacement")
	ErrMissingAVUMutator     = errors.New("missing avu mutator")
)

// AVUReplacement describes an iRODS AVU replacement. iRODS identifies the
// source AVU by name, value, and units; AVU IDs are intentionally not part of
// the replacement contract.
type AVUReplacement struct {
	From AVUStat
	To   AVUStat
}

// AVUMutator is the minimal metadata mutation API required for AVU replacement.
type AVUMutator interface {
	ReplacePathAVU(irodsPath string, replacement AVUReplacement) (AVUStat, error)
}

// MutationService validates and applies metadata mutations through an AVUMutator.
type MutationService struct {
	mutator AVUMutator
}

// NewMutationService creates a metadata mutation service.
func NewMutationService(mutator AVUMutator) (*MutationService, error) {
	if mutator == nil {
		return nil, ErrMissingAVUMutator
	}

	return &MutationService{mutator: mutator}, nil
}

// ReplacePathAVU replaces one AVU on an iRODS path and returns the replacement row.
func (service *MutationService) ReplacePathAVU(irodsPath string, replacement AVUReplacement) (AVUStat, error) {
	irodsPath = normalizeIRODSPath(irodsPath)
	if irodsPath == "" {
		return AVUStat{}, ErrInvalidIRODSPath
	}

	normalized, err := NormalizeAVUReplacement(replacement)
	if err != nil {
		return AVUStat{}, err
	}

	updated, err := service.mutator.ReplacePathAVU(irodsPath, normalized)
	if err != nil {
		return AVUStat{}, fmt.Errorf("replace AVU for %q: %w", irodsPath, err)
	}
	return updated, nil
}

// NormalizeAVUReplacement normalizes AVU names and validates the source and target AVUs.
func NormalizeAVUReplacement(replacement AVUReplacement) (AVUReplacement, error) {
	normalized := AVUReplacement{
		From: normalizeAVUStat(replacement.From),
		To:   normalizeAVUStat(replacement.To),
	}

	if normalized.From.Name == "" || normalized.From.Value == "" {
		return AVUReplacement{}, fmt.Errorf("%w: from name and value are required", ErrInvalidAVUReplacement)
	}
	if normalized.To.Name == "" || normalized.To.Value == "" {
		return AVUReplacement{}, fmt.Errorf("%w: to name and value are required", ErrInvalidAVUReplacement)
	}

	return normalized, nil
}

func normalizeAVUStat(avu AVUStat) AVUStat {
	avu.Name = strings.TrimSpace(avu.Name)
	return avu
}
