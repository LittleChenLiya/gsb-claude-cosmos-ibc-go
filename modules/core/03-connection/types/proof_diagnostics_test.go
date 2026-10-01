// SPDX-License-Identifier: Apache-2.0

package types_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/ibc-go/v11/modules/core/03-connection/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
)

func TestClassifyProofFailure(t *testing.T) {
	testCases := []struct {
		name        string
		err         error
		expCategory types.ProofFailureCategory
	}{
		{
			"no failure",
			nil,
			types.ProofFailureCategoryNone,
		},
		{
			"invalid counterparty prefix",
			errorsmod.Wrap(commitmenttypes.ErrInvalidPrefix, "prefix can't be empty"),
			types.ProofFailureCategoryPrefixMismatch,
		},
		{
			"prefix mismatch at higher subtree",
			errorsmod.Wrap(commitmenttypes.ErrInvalidProof, "failed to verify membership proof at index 1: provided key doesn't match proof"),
			types.ProofFailureCategoryPrefixMismatch,
		},
		{
			"path mismatch at lowest subtree",
			errorsmod.Wrap(commitmenttypes.ErrInvalidProof, "failed to verify membership proof at index 0: provided key doesn't match proof"),
			types.ProofFailureCategoryPathMismatch,
		},
		{
			"root hash mismatch",
			errorsmod.Wrap(commitmenttypes.ErrInvalidProof, "proof did not commit to expected root: AB, got: CD. Please ensure proof was submitted with correct proofHeight and to the correct chain."),
			types.ProofFailureCategoryRootHashMismatch,
		},
		{
			"value mismatch is unknown",
			errorsmod.Wrap(commitmenttypes.ErrInvalidProof, "failed to verify membership proof at index 0: provided value doesn't match proof"),
			types.ProofFailureCategoryUnknown,
		},
		{
			"unrelated error is unknown",
			errors.New("some other error"),
			types.ProofFailureCategoryUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expCategory, types.ClassifyProofFailure(tc.err))
			// errors without a structured category are classified on a best-effort basis
			require.Equal(t, tc.expCategory, types.GetProofFailureCategory(tc.err))
		})
	}
}

func TestProofVerificationError(t *testing.T) {
	innerErr := errorsmod.Wrap(commitmenttypes.ErrInvalidProof, "proof did not commit to expected root: AB, got: CD")

	pvErr := types.NewProofVerificationError(innerErr, "07-tendermint-0", "connection-0")

	require.Equal(t, types.ProofFailureCategoryRootHashMismatch, pvErr.Category)
	require.Equal(t, "07-tendermint-0", pvErr.ClientID)
	require.Equal(t, "connection-0", pvErr.ConnectionID)

	// the structured sentinel and the underlying error chain are both matchable
	require.ErrorIs(t, pvErr, types.ErrConnectionProofVerification)
	require.ErrorIs(t, pvErr, commitmenttypes.ErrInvalidProof)

	// the category is extractable by callers
	require.Equal(t, types.ProofFailureCategoryRootHashMismatch, types.GetProofFailureCategory(pvErr))

	// the category is part of the error text
	require.Contains(t, pvErr.Error(), string(types.ProofFailureCategoryRootHashMismatch))

	// unwrapping returns the underlying error
	require.Equal(t, innerErr, errors.Unwrap(pvErr))
	require.Equal(t, innerErr.Error(), fmt.Sprintf("%s", pvErr.Unwrap()))
}
