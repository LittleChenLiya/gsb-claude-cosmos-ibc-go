// SPDX-License-Identifier: Apache-2.0

package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
)

func TestFreezeInfo_IsEmptyAndString(t *testing.T) {
	// an unfrozen client yields an empty explanation
	empty := types.FreezeInfo{}
	require.True(t, empty.IsEmpty())
	require.Empty(t, empty.String())

	freezeInfo := types.FreezeInfo{
		FrozenHeight:       types.NewHeight(0, 1),
		ConflictingHeights: []types.Height{types.NewHeight(1, 3)},
		EvidenceType:       "ibc.lightclients.tendermint.v1.Header",
	}
	require.False(t, freezeInfo.IsEmpty())

	explanation := freezeInfo.String()
	require.Contains(t, explanation, "0-1")
	require.Contains(t, explanation, "ibc.lightclients.tendermint.v1.Header")
	require.Contains(t, explanation, "1-3")
}
