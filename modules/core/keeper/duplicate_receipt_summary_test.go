// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	channeltypes "github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v11/modules/core/24-host"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
	ibcmock "github.com/cosmos/ibc-go/v11/testing/mock"
)

// TestDuplicatePacketReceiptSummary verifies the read-only duplicate receive
// receipt summary feature:
//
//  1. A mock module sends the same packet twice: the first relay executes the
//     application callback, the duplicate relay returns NOOP and MUST NOT
//     re-trigger the application callback.
//  2. A packet with a new sequence is relayed once: it executes the callback
//     and MUST NOT share its receipt with the previously processed sequence.
//
// It runs for both UNORDERED and ORDERED channels and checks the state
// (receipt/acknowledgement/nextSequenceRecv), the receipt summary returned by
// the read-only keeper API, and the exact number of application callbacks.
func (s *KeeperTestSuite) TestDuplicatePacketReceiptSummary() {
	testCases := []struct {
		name      string
		orderedFn func(*ibctesting.Path)
	}{
		{"UNORDERED channel", func(path *ibctesting.Path) {}},
		{"ORDERED channel", func(path *ibctesting.Path) { path.SetChannelOrdered() }},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			s.SetupTest()
			path := ibctesting.NewPath(s.chainA, s.chainB)
			tc.orderedFn(path)
			path.Setup()

			// application callback counter, shared across all relays in this test case.
			// The callback returns sequence-dependent acknowledgements so that the
			// committed ack hashes for different sequences are distinguishable.
			var (
				callbackCount int
				err           error
				seq1, seq2    uint64
			)
			s.chainB.GetSimApp().IBCMockModule.IBCApp.OnRecvPacket = func(
				ctx sdk.Context, channelVersion string, packet channeltypes.Packet, relayer sdk.AccAddress,
			) exported.Acknowledgement {
				callbackCount++
				if packet.GetSequence() == seq1 {
					return ibcmock.MockAcknowledgement
				}
				return ibcmock.MockFailAcknowledgement
			}

			channelKeeper := s.chainB.App.GetIBCKeeper().ChannelKeeper
			destPort := path.EndpointB.ChannelConfig.PortID
			destChannel := path.EndpointB.ChannelID

			// relayPacket constructs a MsgRecvPacket with a fresh proof from chainA
			// and submits it directly to the core IBC msg server.
			relayPacket := func(packet channeltypes.Packet) (*channeltypes.MsgRecvPacketResponse, error) {
				packetKey := host.PacketCommitmentKey(packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				proof, proofHeight := path.EndpointA.QueryProof(packetKey)
				msg := channeltypes.NewMsgRecvPacket(packet, proof, proofHeight, s.chainB.SenderAccount.GetAddress().String())
				return s.chainB.App.GetIBCKeeper().RecvPacket(s.chainB.GetContext(), msg)
			}

			// send two packets from chainA with sequences 1 and 2
			seq1, err = path.EndpointA.SendPacket(timeoutHeight, 0, ibctesting.MockPacketData)
			s.Require().NoError(err)
			s.Require().Equal(uint64(1), seq1)
			seq2, err = path.EndpointA.SendPacket(timeoutHeight, 0, ibctesting.MockPacketData)
			s.Require().NoError(err)
			s.Require().Equal(uint64(2), seq2)

			packet1 := channeltypes.NewPacket(ibctesting.MockPacketData, seq1, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, destPort, destChannel, timeoutHeight, 0)
			packet2 := channeltypes.NewPacket(ibctesting.MockPacketData, seq2, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, destPort, destChannel, timeoutHeight, 0)

			channel, found := channelKeeper.GetChannel(s.chainB.GetContext(), destPort, destChannel)
			s.Require().True(found)

			// before any relay, seq1 has no receipt (and seq2 must not share one)
			summaryBefore := channelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), packet1, channel)
			s.Require().False(summaryBefore.Received, "seq1 should not be received before first relay")
			s.Require().False(summaryBefore.HasAcknowledgement)

			// first relay of packet seq1: SUCCESS, application callback runs once
			res1, err := relayPacket(packet1)
			s.Require().NoError(err)
			s.Require().Equal(channeltypes.SUCCESS, res1.Result)
			s.Require().Equal(1, callbackCount, "callback should run exactly once on first relay")

			// state after first relay: receipt + synchronous acknowledgement present
			ack1, found := channelKeeper.GetPacketAcknowledgement(s.chainB.GetContext(), destPort, destChannel, seq1)
			s.Require().True(found)
			s.Require().NotEmpty(ack1)
			if channel.Ordering == channeltypes.UNORDERED {
				_, found = channelKeeper.GetPacketReceipt(s.chainB.GetContext(), destPort, destChannel, seq1)
				s.Require().True(found, "UNORDERED receipt for seq1 must exist")
			}

			// read-only summary for the already processed packet
			summary1 := channelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), packet1, channel)
			s.Require().True(summary1.Received, "summary must report seq1 as received")
			s.Require().True(summary1.HasAcknowledgement, "summary must report the ack for seq1")
			s.Require().Equal(ack1, summary1.AcknowledgementHash, "summary must carry the ack hash of seq1")
			s.Require().Equal(destPort, summary1.DestinationPort)
			s.Require().Equal(destChannel, summary1.DestinationChannel)
			s.Require().Equal(seq1, summary1.Sequence)
			s.Require().Equal(channel.Ordering, summary1.Ordering)
			if channel.Ordering == channeltypes.ORDERED {
				s.Require().Equal(uint64(2), summary1.NextSequenceRecv)
			} else {
				s.Require().Equal(uint64(0), summary1.NextSequenceRecv)
			}

			// duplicate relay of the SAME packet: NOOP, callback count unchanged
			resDup1, err := relayPacket(packet1)
			s.Require().NoError(err)
			s.Require().Equal(channeltypes.NOOP, resDup1.Result, "duplicate relay must be a no-op")
			s.Require().Equal(1, callbackCount, "duplicate relay must not re-trigger the application callback")

			// third attempt, same packet again: still NOOP, callback count still 1
			resDup1Again, err := relayPacket(packet1)
			s.Require().NoError(err)
			s.Require().Equal(channeltypes.NOOP, resDup1Again.Result)
			s.Require().Equal(1, callbackCount, "second duplicate relay must not re-trigger the application callback")

			// the duplicate relays must not have mutated the seq1 state
			ack1After, found := channelKeeper.GetPacketAcknowledgement(s.chainB.GetContext(), destPort, destChannel, seq1)
			s.Require().True(found)
			s.Require().Equal(ack1, ack1After, "duplicate relays must not overwrite seq1 acknowledgement")
			summary1After := channelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), packet1, channel)
			s.Require().Equal(summary1, summary1After, "summary must be deterministic across duplicate relays")

			// boundary: different sequence (seq2) must NOT share seq1 receipt
			res2, err := relayPacket(packet2)
			s.Require().NoError(err)
			s.Require().Equal(channeltypes.SUCCESS, res2.Result, "new sequence must be processed normally")
			s.Require().Equal(2, callbackCount, "callback should run once for the new sequence")

			summary2 := channelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), packet2, channel)
			s.Require().True(summary2.Received)
			s.Require().True(summary2.HasAcknowledgement)
			s.Require().Equal(seq2, summary2.Sequence)
			s.Require().NotEqual(summary1.AcknowledgementHash, summary2.AcknowledgementHash, "acknowledgement hashes must not be shared across sequences")

			if channel.Ordering == channeltypes.UNORDERED {
				_, found = channelKeeper.GetPacketReceipt(s.chainB.GetContext(), destPort, destChannel, seq2)
				s.Require().True(found, "UNORDERED receipt for seq2 must exist independently")
			} else {
				nextSeqRecv, found := channelKeeper.GetNextSequenceRecv(s.chainB.GetContext(), destPort, destChannel)
				s.Require().True(found)
				s.Require().Equal(uint64(3), nextSeqRecv, "ORDERED nextSequenceRecv must advance to 3")
			}

			// replaying seq1 once more after seq2 was processed must still be a NOOP
			resDup1Late, err := relayPacket(packet1)
			s.Require().NoError(err)
			s.Require().Equal(channeltypes.NOOP, resDup1Late.Result)
			s.Require().Equal(2, callbackCount, "late duplicate of seq1 must not trigger callback")
		})
	}
}
