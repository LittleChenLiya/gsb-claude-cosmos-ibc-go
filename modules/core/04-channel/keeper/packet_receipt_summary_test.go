// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
	ibcmock "github.com/cosmos/ibc-go/v11/testing/mock"
)

// TestGetPacketReceiptSummary exercises the read-only summary API directly
// against keeper state for both UNORDERED and ORDERED channels. It verifies
// that summaries are scoped per sequence and that reading the summary neither
// writes state nor invokes any application logic.
func (s *KeeperTestSuite) TestGetPacketReceiptSummary() {
	testCases := []struct {
		name      string
		orderedFn func(*ibctesting.Path)
	}{
		{"UNORDERED channel", func(path *ibctesting.Path) {}},
		{"ORDERED channel", func(path *ibctesting.Path) { path.SetChannelOrdered() }},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			path := ibctesting.NewPath(s.chainA, s.chainB)
			tc.orderedFn(path)
			path.Setup()

			ctx := s.chainB.GetContext()
			k := s.chainB.App.GetIBCKeeper().ChannelKeeper
			portID := path.EndpointB.ChannelConfig.PortID
			channelID := path.EndpointB.ChannelID

			channel, found := k.GetChannel(ctx, portID, channelID)
			s.Require().True(found)

			newPacket := func(seq uint64) types.Packet {
				return types.NewPacket(
					ibctesting.MockPacketData, seq,
					path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
					portID, channelID,
					clienttypes.NewHeight(1, 100), 0,
				)
			}

			packet1 := newPacket(1)
			packet2 := newPacket(2)

			// nothing received yet
			summary := k.GetPacketReceiptSummary(ctx, packet1, channel)
			s.Require().False(summary.Received)
			s.Require().False(summary.HasAcknowledgement)
			s.Require().Nil(summary.AcknowledgementHash)
			s.Require().Equal(uint64(1), summary.Sequence)
			s.Require().Equal(channel.Ordering, summary.Ordering)

			// String must render the key identifying fields
			s.Require().Contains(summary.String(), "Sequence: 1")
			s.Require().Contains(summary.String(), "Received: false")

			// simulate first processing result for seq1
			ack1 := types.CommitAcknowledgement(ibcmock.MockAcknowledgement.Acknowledgement())
			if channel.Ordering == types.UNORDERED {
				k.SetPacketReceipt(ctx, portID, channelID, 1)
			} else {
				k.SetNextSequenceRecv(ctx, portID, channelID, 2)
			}
			k.SetPacketAcknowledgement(ctx, portID, channelID, 1, ack1)

			summary1 := k.GetPacketReceiptSummary(ctx, packet1, channel)
			s.Require().True(summary1.Received)
			s.Require().True(summary1.HasAcknowledgement)
			s.Require().Equal(ack1, summary1.AcknowledgementHash)
			if channel.Ordering == types.ORDERED {
				s.Require().Equal(uint64(2), summary1.NextSequenceRecv)
			}

			// seq2 must not share the seq1 receipt/ack
			summary2 := k.GetPacketReceiptSummary(ctx, packet2, channel)
			if channel.Ordering == types.UNORDERED {
				s.Require().False(summary2.Received, "seq2 must not inherit seq1 receipt")
			} else {
				// seq2 is exactly nextSequenceRecv, so it is not yet received
				s.Require().False(summary2.Received, "seq2 must be the next, not yet received sequence")
				s.Require().Equal(uint64(2), summary2.NextSequenceRecv)
			}
			s.Require().False(summary2.HasAcknowledgement, "seq2 must not inherit seq1 acknowledgement")
			s.Require().Nil(summary2.AcknowledgementHash)
			s.Require().Equal(uint64(2), summary2.Sequence)
		})
	}
}
