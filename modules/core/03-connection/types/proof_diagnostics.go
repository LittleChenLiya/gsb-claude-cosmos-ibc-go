// SPDX-License-Identifier: Apache-2.0

package types

import (
	"errors"
	"fmt"
	"strings"

	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
)

// ProofFailureCategory is a structured classification of why connection proof
// verification failed. It allows callers to diagnose verification failures
// programmatically without parsing error strings.
type ProofFailureCategory string

const (
	// ProofFailureCategoryNone indicates that no failure occurred.
	ProofFailureCategoryNone ProofFailureCategory = ""
	// ProofFailureCategoryPrefixMismatch indicates that the counterparty
	// commitment prefix does not match the store key the proof commits to.
	ProofFailureCategoryPrefixMismatch ProofFailureCategory = "prefix_mismatch"
	// ProofFailureCategoryPathMismatch indicates that the commitment path
	// (e.g. the connection identifier) does not match the key the proof
	// commits to.
	ProofFailureCategoryPathMismatch ProofFailureCategory = "path_mismatch"
	// ProofFailureCategoryRootHashMismatch indicates that the proof commits to
	// a different root hash than the consensus state root it is verified
	// against.
	ProofFailureCategoryRootHashMismatch ProofFailureCategory = "root_hash_mismatch"
	// ProofFailureCategoryUnknown indicates that the failure could not be
	// classified into a more specific category.
	ProofFailureCategoryUnknown ProofFailureCategory = "unknown"
)

// String implements fmt.Stringer.
func (c ProofFailureCategory) String() string {
	return string(c)
}

// ProofVerificationError is a structured error returned when connection proof
// verification fails. It carries a machine readable ProofFailureCategory so
// that callers can diagnose the failure without parsing error strings. The
// full proof bytes are never included in the error text.
type ProofVerificationError struct {
	// Category is the structured classification of the failure.
	Category ProofFailureCategory
	// ClientID is the identifier of the client the proof was verified against.
	ClientID string
	// ConnectionID is the identifier of the counterparty connection being proven.
	ConnectionID string
	// Err is the underlying verification error.
	Err error
}

// NewProofVerificationError returns a ProofVerificationError for the given
// verification error. The failure category is derived from the error via
// ClassifyProofFailure.
func NewProofVerificationError(err error, clientID, connectionID string) *ProofVerificationError {
	return &ProofVerificationError{
		Category:     ClassifyProofFailure(err),
		ClientID:     clientID,
		ConnectionID: connectionID,
		Err:          err,
	}
}

// Error implements the error interface. It never includes the full proof bytes.
func (e *ProofVerificationError) Error() string {
	return fmt.Sprintf("%s (failure category: %s)", e.Err, e.Category)
}

// Unwrap implements the built-in error unwrapping interface.
func (e *ProofVerificationError) Unwrap() error {
	return e.Err
}

// Is reports whether the target matches ErrConnectionProofVerification or any
// error in the underlying error chain.
func (e *ProofVerificationError) Is(target error) bool {
	return target == ErrConnectionProofVerification || errors.Is(e.Err, target)
}

// GetProofFailureCategory extracts the structured failure category from an
// error returned by connection proof verification. Errors that do not carry a
// ProofVerificationError are classified on a best-effort basis via
// ClassifyProofFailure.
func GetProofFailureCategory(err error) ProofFailureCategory {
	var pvErr *ProofVerificationError
	if errors.As(err, &pvErr) {
		return pvErr.Category
	}

	return ClassifyProofFailure(err)
}

// ClassifyProofFailure inspects a connection proof verification error and
// returns its structured failure category, distinguishing prefix, path and
// root hash mismatches. A nil error classifies as ProofFailureCategoryNone.
func ClassifyProofFailure(err error) ProofFailureCategory {
	if err == nil {
		return ProofFailureCategoryNone
	}

	// an invalid counterparty prefix is detected before proof verification runs
	if errors.Is(err, commitmenttypes.ErrInvalidPrefix) {
		return ProofFailureCategoryPrefixMismatch
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "proof did not commit to expected root"):
		// the proof is internally valid but commits to a different root hash
		// than the consensus state root it is verified against
		return ProofFailureCategoryRootHashMismatch
	case strings.Contains(msg, "failed to verify membership proof at index 0: provided key doesn't match proof"):
		// the lowest subtree proof commits to a different key, i.e. the
		// commitment path (e.g. the connection identifier) does not match
		return ProofFailureCategoryPathMismatch
	case strings.Contains(msg, "failed to verify membership proof at index") && strings.Contains(msg, "provided key doesn't match proof"):
		// a higher subtree proof commits to a different key, i.e. the
		// top-level store key derived from the counterparty prefix does
		// not match
		return ProofFailureCategoryPrefixMismatch
	default:
		return ProofFailureCategoryUnknown
	}
}
