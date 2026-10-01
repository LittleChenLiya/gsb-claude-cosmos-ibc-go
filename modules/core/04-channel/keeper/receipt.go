// SPDX-License-Identifier: Apache-2.0

package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
)

// GetPacketReceiptSummary returns a read-only summary of the prior processing
// result for a packet on its destination channel. It performs no state
// mutation and must be used on the duplicate packet receive path to confirm
// the outcome of the earlier processing.
//
// The summary is keyed by the packet's destination port/channel and sequence,
// so distinct sequences never share receipts: a receipt for one sequence does
// not imply that a different sequence was received.
func (k *Keeper) GetPacketReceiptSummary(ctx sdk.Context, packet types.Packet) types.PacketReceiptSummary {
	summary := types.PacketReceiptSummary{
		Sequence:      packet.GetSequence(),
		SourcePort:    packet.GetSourcePort(),
		SourceChannel: packet.GetSourceChannel(),
		DestPort:      packet.GetDestPort(),
		DestChannel:   packet.GetDestChannel(),
	}

	channel, found := k.GetChannel(ctx, packet.GetDestPort(), packet.GetDestChannel())

	// On ordered channels receipts are not stored; the next receive sequence
	// determines whether the packet has already been processed.
	if found && channel.Ordering == types.ORDERED {
		nextSequenceRecv, found := k.GetNextSequenceRecv(ctx, packet.GetDestPort(), packet.GetDestChannel())
		if found {
			summary.NextSequenceRecv = nextSequenceRecv
			summary.AlreadyReceived = packet.GetSequence() < nextSequenceRecv
		}
	} else {
		// On unordered channels the receipt is the per-sequence marker that a
		// packet has already been received.
		receipt, found := k.GetPacketReceipt(ctx, packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
		summary.AlreadyReceived = found
		if found {
			summary.Receipt = []byte(receipt)
		}
	}

	// The acknowledgement commitment is stored for both orderings once an
	// acknowledgement has been written synchronously.
	if ackCommitment, found := k.GetPacketAcknowledgement(ctx, packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence()); found {
		summary.AcknowledgementCommitment = ackCommitment
	}

	return summary
}
