// SPDX-License-Identifier: Apache-2.0

package keeper

import (
	"bytes"
	"math"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/03-connection/types"
	channeltypes "github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
	commitmenttypesv2 "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types/v2"
	ibcerrors "github.com/cosmos/ibc-go/v11/modules/core/errors"
	host "github.com/cosmos/ibc-go/v11/modules/core/24-host"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
)

// VerifyConnectionState verifies a proof of the connection state of the
// specified connection end stored on the target machine.
func (k *Keeper) VerifyConnectionState(
	ctx sdk.Context,
	connection types.ConnectionEnd,
	height exported.Height,
	proof []byte,
	connectionID string,
	counterpartyConnection types.ConnectionEnd, // opposite connection
) error {
	clientID := connection.ClientId
	merklePath := commitmenttypes.NewMerklePath(host.ConnectionKey(connectionID))
	merklePath, err := commitmenttypes.ApplyPrefix(connection.Counterparty.Prefix, merklePath)
	if err != nil {
		return errorsmod.Wrap(err, "failed to apply counterparty prefix")
	}

	bz, err := k.cdc.Marshal(&counterpartyConnection)
	if err != nil {
		return err
	}

	if err := k.clientKeeper.VerifyMembership(
		ctx, clientID, height,
		0, 0, // skip delay period checks for non-packet processing verification
		proof, merklePath, bz,
	); err != nil {
		category := k.diagnoseConnectionProofFailure(ctx, clientID, height, proof, merklePath, bz, err)
		return types.NewProofVerificationError(category, clientID, height, err)
	}

	return nil
}

// VerifyChannelState verifies a proof of the channel state of the specified
// channel end, under the specified port, stored on the target machine.
func (k *Keeper) VerifyChannelState(
	ctx sdk.Context,
	connection types.ConnectionEnd,
	height exported.Height,
	proof []byte,
	portID,
	channelID string,
	channel channeltypes.Channel,
) error {
	clientID := connection.ClientId
	merklePath := commitmenttypes.NewMerklePath(host.ChannelKey(portID, channelID))
	merklePath, err := commitmenttypes.ApplyPrefix(connection.Counterparty.Prefix, merklePath)
	if err != nil {
		return err
	}

	bz, err := k.cdc.Marshal(&channel)
	if err != nil {
		return err
	}

	if err := k.clientKeeper.VerifyMembership(
		ctx, clientID, height,
		0, 0, // skip delay period checks for non-packet processing verification
		proof, merklePath, bz,
	); err != nil {
		return errorsmod.Wrapf(err, "failed channel state verification for client (%s)", clientID)
	}

	return nil
}

// VerifyPacketCommitment verifies a proof of an outgoing packet commitment at
// the specified port, specified channel, and specified sequence.
func (k *Keeper) VerifyPacketCommitment(
	ctx sdk.Context,
	connection types.ConnectionEnd,
	height exported.Height,
	proof []byte,
	portID,
	channelID string,
	sequence uint64,
	commitmentBytes []byte,
) error {
	clientID := connection.ClientId
	// get time and block delays
	timeDelay := connection.DelayPeriod
	blockDelay := k.getBlockDelay(ctx, connection)

	merklePath := commitmenttypes.NewMerklePath(host.PacketCommitmentKey(portID, channelID, sequence))
	merklePath, err := commitmenttypes.ApplyPrefix(connection.Counterparty.Prefix, merklePath)
	if err != nil {
		return err
	}

	if err := k.clientKeeper.VerifyMembership(
		ctx, clientID, height, timeDelay, blockDelay, proof, merklePath, commitmentBytes,
	); err != nil {
		return errorsmod.Wrapf(err, "failed packet commitment verification for client (%s)", clientID)
	}

	return nil
}

// VerifyPacketAcknowledgement verifies a proof of an incoming packet
// acknowledgement at the specified port, specified channel, and specified sequence.
func (k *Keeper) VerifyPacketAcknowledgement(
	ctx sdk.Context,
	connection types.ConnectionEnd,
	height exported.Height,
	proof []byte,
	portID,
	channelID string,
	sequence uint64,
	acknowledgement []byte,
) error {
	clientID := connection.ClientId
	// get time and block delays
	timeDelay := connection.DelayPeriod
	blockDelay := k.getBlockDelay(ctx, connection)

	merklePath := commitmenttypes.NewMerklePath(host.PacketAcknowledgementKey(portID, channelID, sequence))
	merklePath, err := commitmenttypes.ApplyPrefix(connection.Counterparty.Prefix, merklePath)
	if err != nil {
		return err
	}

	if err := k.clientKeeper.VerifyMembership(
		ctx, clientID, height, timeDelay, blockDelay,
		proof, merklePath, channeltypes.CommitAcknowledgement(acknowledgement),
	); err != nil {
		return errorsmod.Wrapf(err, "failed packet acknowledgement verification for client (%s)", clientID)
	}

	return nil
}

// VerifyPacketReceiptAbsence verifies a proof of the absence of an
// incoming packet receipt at the specified port, specified channel, and
// specified sequence.
func (k *Keeper) VerifyPacketReceiptAbsence(
	ctx sdk.Context,
	connection types.ConnectionEnd,
	height exported.Height,
	proof []byte,
	portID,
	channelID string,
	sequence uint64,
) error {
	clientID := connection.ClientId
	// get time and block delays
	timeDelay := connection.DelayPeriod
	blockDelay := k.getBlockDelay(ctx, connection)

	merklePath := commitmenttypes.NewMerklePath(host.PacketReceiptKey(portID, channelID, sequence))
	merklePath, err := commitmenttypes.ApplyPrefix(connection.Counterparty.Prefix, merklePath)
	if err != nil {
		return err
	}

	if err := k.clientKeeper.VerifyNonMembership(
		ctx, clientID, height, timeDelay, blockDelay, proof, merklePath,
	); err != nil {
		return errorsmod.Wrapf(err, "failed packet receipt absence verification for client (%s)", clientID)
	}

	return nil
}

// VerifyNextSequenceRecv verifies a proof of the next sequence number to be
// received of the specified channel at the specified port.
func (k *Keeper) VerifyNextSequenceRecv(
	ctx sdk.Context,
	connection types.ConnectionEnd,
	height exported.Height,
	proof []byte,
	portID,
	channelID string,
	nextSequenceRecv uint64,
) error {
	clientID := connection.ClientId
	// get time and block delays
	timeDelay := connection.DelayPeriod
	blockDelay := k.getBlockDelay(ctx, connection)

	merklePath := commitmenttypes.NewMerklePath(host.NextSequenceRecvKey(portID, channelID))
	merklePath, err := commitmenttypes.ApplyPrefix(connection.Counterparty.Prefix, merklePath)
	if err != nil {
		return err
	}

	if err := k.clientKeeper.VerifyMembership(
		ctx, clientID, height,
		timeDelay, blockDelay,
		proof, merklePath, sdk.Uint64ToBigEndian(nextSequenceRecv),
	); err != nil {
		return errorsmod.Wrapf(err, "failed next sequence receive verification for client (%s)", clientID)
	}

	return nil
}

// getBlockDelay calculates the block delay period from the time delay of the connection
// and the maximum expected time per block.
func (k *Keeper) getBlockDelay(ctx sdk.Context, connection types.ConnectionEnd) uint64 {
	// expectedTimePerBlock should never be zero, however if it is then return a 0 block delay for safety
	// as the expectedTimePerBlock parameter was not set.
	expectedTimePerBlock := k.GetParams(ctx).MaxExpectedTimePerBlock
	if expectedTimePerBlock == 0 {
		return 0
	}
	// calculate minimum block delay by dividing time delay period
	// by the expected time per block. Round up the block delay.
	timeDelay := connection.DelayPeriod
	return uint64(math.Ceil(float64(timeDelay) / float64(expectedTimePerBlock)))
}

// consensusRootProvider is an optional interface that a consensus state may
// implement to expose its commitment root.
type consensusRootProvider interface {
	GetRoot() exported.Root
}

// diagnoseConnectionProofFailure determines the structured failure category of
// a failed connection proof verification. It inspects the proof, the merkle
// path, and the consensus state root to distinguish prefix, path, and root
// hash mismatches without relying on error text. The raw proof bytes are never
// included in the returned category.
func (k *Keeper) diagnoseConnectionProofFailure(
	ctx sdk.Context,
	clientID string,
	height exported.Height,
	proof []byte,
	merklePath commitmenttypesv2.MerklePath,
	value []byte,
	verifyErr error,
) types.ProofVerificationCategory {
	// Handle well-defined sentinel errors first.
	switch {
	case errorsmod.IsOf(verifyErr, ibcerrors.ErrInvalidHeight):
		return types.ProofVerificationCategoryHeightMismatch
	case errorsmod.IsOf(verifyErr, clienttypes.ErrClientNotActive):
		return types.ProofVerificationCategoryClientInactive
	case errorsmod.IsOf(verifyErr, clienttypes.ErrConsensusStateNotFound):
		return types.ProofVerificationCategoryConsensusStateNotFound
	}

	// Unmarshal the proof so we can inspect its structure.
	var merkleProof commitmenttypes.MerkleProof
	if err := k.cdc.Unmarshal(proof, &merkleProof); err != nil {
		return types.ProofVerificationCategoryMalformedProof
	}

	// Retrieve the consensus state root for the proof height.
	consensusState, found := k.clientKeeper.GetClientConsensusState(ctx, clientID, height)
	if !found {
		return types.ProofVerificationCategoryConsensusStateNotFound
	}
	rootProvider, ok := consensusState.(consensusRootProvider)
	if !ok {
		return types.ProofVerificationCategoryUnknown
	}
	root := rootProvider.GetRoot()
	if root == nil || root.Empty() {
		return types.ProofVerificationCategoryMalformedProof
	}

	// The number of path elements must match the number of proof levels.
	if len(merklePath.KeyPath) != len(merkleProof.Proofs) {
		return types.ProofVerificationCategoryMalformedProof
	}

	// Check for key mismatches at each proof level. The path is ordered from
	// highest (prefix) to lowest (connection key), while the proofs are
	// ordered from lowest (IAVL) to highest (tendermint). A mismatch at the
	// highest proof level indicates a prefix mismatch; a mismatch at any lower
	// level indicates a path mismatch.
	numLevels := len(merkleProof.Proofs)
	for i := 0; i < numLevels; i++ {
		existenceProof := merkleProof.Proofs[i].GetExist()
		if existenceProof == nil {
			return types.ProofVerificationCategoryMalformedProof
		}
		pathKey := merklePath.KeyPath[numLevels-1-i]
		if !bytes.Equal(existenceProof.Key, pathKey) {
			if i == numLevels-1 {
				return types.ProofVerificationCategoryPrefixMismatch
			}
			return types.ProofVerificationCategoryPathMismatch
		}
	}

	// Check for a value mismatch at the lowest proof level.
	lowestProof := merkleProof.Proofs[0].GetExist()
	if lowestProof != nil && !bytes.Equal(lowestProof.Value, value) {
		return types.ProofVerificationCategoryValueMismatch
	}

	// Calculate the root committed to by the proof chain and compare it with
	// the consensus state root.
	calculatedRoot, err := merkleProof.Proofs[numLevels-1].Calculate()
	if err != nil {
		return types.ProofVerificationCategoryMalformedProof
	}
	if !bytes.Equal(root.GetHash(), calculatedRoot) {
		return types.ProofVerificationCategoryRootMismatch
	}

	return types.ProofVerificationCategoryUnknown
}
