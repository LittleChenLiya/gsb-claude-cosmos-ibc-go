// SPDX-License-Identifier: Apache-2.0

package types

import (
	"errors"
	"fmt"

	"github.com/cosmos/ibc-go/v11/modules/core/exported"
)

// ProofVerificationCategory classifies why a connection proof verification
// failed. The category is derived structurally from the proof, path, and
// consensus state so that callers can distinguish the root cause without
// parsing error text.
type ProofVerificationCategory uint8

const (
	// ProofVerificationCategoryUnknown indicates the failure could not be
	// categorized into one of the well-defined categories.
	ProofVerificationCategoryUnknown ProofVerificationCategory = iota
	// ProofVerificationCategoryPrefixMismatch indicates the counterparty
	// prefix on the connection end does not match the prefix the proof was
	// generated against.
	ProofVerificationCategoryPrefixMismatch
	// ProofVerificationCategoryPathMismatch indicates the connection path
	// (connection identifier) does not match the path the proof was generated
	// against.
	ProofVerificationCategoryPathMismatch
	// ProofVerificationCategoryRootMismatch indicates the proof was generated
	// against a consensus state root that differs from the root stored on
	// chain for the proof height.
	ProofVerificationCategoryRootMismatch
	// ProofVerificationCategoryValueMismatch indicates the connection state
	// value committed to by the proof does not match the expected connection
	// end.
	ProofVerificationCategoryValueMismatch
	// ProofVerificationCategoryHeightMismatch indicates the proof height is
	// greater than the client's latest height.
	ProofVerificationCategoryHeightMismatch
	// ProofVerificationCategoryClientInactive indicates the light client is
	// not in the active state.
	ProofVerificationCategoryClientInactive
	// ProofVerificationCategoryConsensusStateNotFound indicates no consensus
	// state exists for the proof height on the client.
	ProofVerificationCategoryConsensusStateNotFound
	// ProofVerificationCategoryMalformedProof indicates the proof bytes could
	// not be unmarshaled or are structurally invalid.
	ProofVerificationCategoryMalformedProof
)

// String returns a human-readable description of the proof verification
// category.
func (c ProofVerificationCategory) String() string {
	switch c {
	case ProofVerificationCategoryPrefixMismatch:
		return "prefix mismatch"
	case ProofVerificationCategoryPathMismatch:
		return "path mismatch"
	case ProofVerificationCategoryRootMismatch:
		return "root hash mismatch"
	case ProofVerificationCategoryValueMismatch:
		return "value mismatch"
	case ProofVerificationCategoryHeightMismatch:
		return "height mismatch"
	case ProofVerificationCategoryClientInactive:
		return "client inactive"
	case ProofVerificationCategoryConsensusStateNotFound:
		return "consensus state not found"
	case ProofVerificationCategoryMalformedProof:
		return "malformed proof"
	default:
		return "unknown"
	}
}

// ProofVerificationError is returned when connection proof verification fails.
// It carries a structured category so that callers can programmatically
// distinguish prefix, path, and root hash mismatches without parsing error
// text. The error message never includes the raw proof bytes.
type ProofVerificationError struct {
	Category ProofVerificationCategory
	ClientID string
	Height   exported.Height
	err      error
}

// NewProofVerificationError constructs a new ProofVerificationError.
func NewProofVerificationError(category ProofVerificationCategory, clientID string, height exported.Height, err error) *ProofVerificationError {
	return &ProofVerificationError{
		Category: category,
		ClientID: clientID,
		Height:   height,
		err:      err,
	}
}

// Error implements the error interface. It returns a structured message that
// never includes the raw proof bytes.
func (e *ProofVerificationError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("connection proof verification failed for client (%s) at height (%s): %s: %v", e.ClientID, e.Height, e.Category, e.err)
	}
	return fmt.Sprintf("connection proof verification failed for client (%s) at height (%s): %s", e.ClientID, e.Height, e.Category)
}

// Unwrap returns the underlying error.
func (e *ProofVerificationError) Unwrap() error {
	return e.err
}

// Is allows callers to match the error against a target category sentinel.
func (e *ProofVerificationError) Is(target error) bool {
	t, ok := target.(*ProofVerificationError)
	if !ok {
		return false
	}
	return e.Category == t.Category
}

// CategoryFromError extracts the ProofVerificationCategory from an error. If
// the error is not a ProofVerificationError, ProofVerificationCategoryUnknown
// is returned.
func CategoryFromError(err error) ProofVerificationCategory {
	var pve *ProofVerificationError
	if errors.As(err, &pve) {
		return pve.Category
	}
	return ProofVerificationCategoryUnknown
}
