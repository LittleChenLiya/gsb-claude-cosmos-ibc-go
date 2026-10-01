// SPDX-License-Identifier: Apache-2.0

package keeper

import (
	"encoding/json"

	gogoproto "github.com/cosmos/gogoproto/proto"

	sdk "github.com/cosmos/cosmos-sdk/types"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
)

// frozenHeightProvider is an optional interface implemented by client states
// which expose the height at which the client was frozen. Client states which
// do not implement this interface simply yield an empty frozen height in the
// freeze information.
type frozenHeightProvider interface {
	GetFrozenHeight() clienttypes.Height
}

// conflictingHeightsProvider is an optional interface implemented by client
// messages (e.g. a double-signing Misbehaviour) which can report the heights of
// the conflicting evidence they carry.
type conflictingHeightsProvider interface {
	GetConflictingHeights() []exported.Height
}

// conflictingHeightProvider is an optional interface implemented by client
// messages (e.g. a Header) which carry a single height.
type conflictingHeightProvider interface {
	GetHeight() exported.Height
}

// GetClientFreezeInfo returns a read-only description of why the client
// identified by clientID was frozen, including the frozen height, the
// conflicting heights and the type of evidence that triggered the freeze.
//
// A zero-value (empty) FreezeInfo is returned if the client is not frozen,
// the client cannot be routed, or no freeze information was recorded. This
// method never mutates state.
func (k *Keeper) GetClientFreezeInfo(ctx sdk.Context, clientID string) clienttypes.FreezeInfo {
	if status := k.GetClientStatus(ctx, clientID); status != exported.Frozen {
		// unfrozen (or unavailable) clients have no freeze information
		return clienttypes.FreezeInfo{}
	}

	bz := k.ClientStore(ctx, clientID).Get(clienttypes.FreezeInfoKey())
	if len(bz) == 0 {
		// a client frozen outside of UpdateClient (e.g. migrated state) has no recorded explanation
		return clienttypes.FreezeInfo{}
	}

	var freezeInfo clienttypes.FreezeInfo
	if err := json.Unmarshal(bz, &freezeInfo); err != nil {
		return clienttypes.FreezeInfo{}
	}

	return freezeInfo
}

// deriveFreezeInfo builds the read-only freeze information from the verified
// client message that caused the freeze and from the frozen client state.
// It does not perform any verification itself.
func deriveFreezeInfo(clientMsg exported.ClientMessage, clientState exported.ClientState) clienttypes.FreezeInfo {
	freezeInfo := clienttypes.FreezeInfo{
		EvidenceType: evidenceTypeName(clientMsg),
	}

	if provider, ok := clientState.(frozenHeightProvider); ok {
		freezeInfo.FrozenHeight = provider.GetFrozenHeight()
	}

	switch msg := clientMsg.(type) {
	case conflictingHeightsProvider:
		for _, h := range msg.GetConflictingHeights() {
			if h == nil || h.IsZero() {
				continue
			}
			if height, ok := h.(clienttypes.Height); ok {
				freezeInfo.ConflictingHeights = append(freezeInfo.ConflictingHeights, height)
			}
		}
	case conflictingHeightProvider:
		if h := msg.GetHeight(); h != nil && !h.IsZero() {
			if height, ok := h.(clienttypes.Height); ok {
				freezeInfo.ConflictingHeights = append(freezeInfo.ConflictingHeights, height)
			}
		}
	}

	return freezeInfo
}

// evidenceTypeName returns the protobuf message name of the evidence that
// triggered the freeze. It returns an empty string if the name cannot be resolved.
func evidenceTypeName(clientMsg exported.ClientMessage) string {
	if clientMsg == nil {
		return ""
	}

	return gogoproto.MessageName(clientMsg)
}

// setClientFreezeInfo persists the freeze information in the client-specific store.
func (k *Keeper) setClientFreezeInfo(ctx sdk.Context, clientID string, freezeInfo clienttypes.FreezeInfo) {
	if freezeInfo.IsEmpty() {
		return
	}

	bz, err := json.Marshal(freezeInfo)
	if err != nil {
		// freeze information is auxiliary metadata and must never affect the freeze path
		k.Logger(ctx).Error("failed to marshal client freeze info", "client-id", clientID, "err", err)
		return
	}

	k.ClientStore(ctx, clientID).Set(clienttypes.FreezeInfoKey(), bz)
}

// deleteClientFreezeInfo removes any recorded freeze information for a client.
// It is used when a frozen client is recovered by a substitute client.
func (k *Keeper) deleteClientFreezeInfo(ctx sdk.Context, clientID string) {
	k.ClientStore(ctx, clientID).Delete(clienttypes.FreezeInfoKey())
}
