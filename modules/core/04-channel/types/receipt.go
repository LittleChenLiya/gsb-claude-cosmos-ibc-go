// SPDX-License-Identifier: Apache-2.0

package types

import "fmt"

// PacketReceiptSummary is a read-only summary of the outcome of a previously
// received packet. It is populated on the duplicate packet receive path
// (ErrNoOpMsg) and allows callers to confirm the result of the earlier
// processing without re-executing the application callback.
//
// The summary is keyed by the packet's destination port/channel and sequence:
// receipts are stored per sequence, so two different sequences never share a
// receipt. A duplicate packet (same sequence received again) returns the
// summary of the prior processing instead of triggering the application
// callback a second time.
type PacketReceiptSummary struct {
	// Sequence is the sequence of the packet that was already received.
	Sequence uint64
	// SourcePort is the port on the sending chain.
	SourcePort string
	// SourceChannel is the channel on the sending chain.
	SourceChannel string
	// DestPort is the port on the receiving chain.
	DestPort string
	// DestChannel is the channel on the receiving chain.
	DestChannel string
	// AlreadyReceived indicates whether the packet has already been processed
	// on this chain. For unordered channels this is true iff a receipt exists
	// for the sequence; for ordered channels this is true iff the sequence is
	// strictly smaller than the next receive sequence.
	AlreadyReceived bool
	// NextSequenceRecv is the next sequence expected to be received on an
	// ordered channel. It is zero for unordered channels.
	NextSequenceRecv uint64
	// Receipt is the raw receipt stored for the packet on an unordered
	// channel. It is empty for ordered channels, which do not store receipts.
	Receipt []byte
	// AcknowledgementCommitment is the hash of the acknowledgement written for
	// the packet. It is empty if no acknowledgement has been written (e.g.
	// asynchronous acknowledgements).
	AcknowledgementCommitment []byte
}

// String implements the Stringer interface.
func (s PacketReceiptSummary) String() string {
	return fmt.Sprintf(
		"packet receipt summary: sequence: %d, src port: %s, src channel: %s, dst port: %s, dst channel: %s, already received: %t, next sequence recv: %d, receipt: %X, ack commitment: %X",
		s.Sequence, s.SourcePort, s.SourceChannel, s.DestPort, s.DestChannel, s.AlreadyReceived, s.NextSequenceRecv, s.Receipt, s.AcknowledgementCommitment,
	)
}
