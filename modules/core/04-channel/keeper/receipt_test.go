// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v11/modules/core/24-host"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
	ibcmock "github.com/cosmos/ibc-go/v11/testing/mock"
)

// TestRecvPacketDuplicateReceiptSummary tests that a duplicate packet receive
// path returns a read-only receipt summary of the prior processing result,
// while the application callback is only invoked once per distinct sequence.
//
// The mock module is used to count application callback invocations: the same
// packet is sent twice and a new sequence once. The callback must fire exactly
// once per distinct sequence (twice total), and never for the duplicate.
func (s *KeeperTestSuite) TestRecvPacketDuplicateReceiptSummary() {
	var (
		path          *ibctesting.Path
		callbackCount uint64
	)

	// setup uses an UNORDERED channel
	path = ibctesting.NewPath(s.chainA, s.chainB)
	path.Setup()

	// override the mock application callback to count invocations and return
	// a successful acknowledgement.
	s.chainB.GetSimApp().IBCMockModule.IBCApp.OnRecvPacket = func(
		ctx sdk.Context, channelVersion string, packet types.Packet, relayer sdk.AccAddress,
	) exported.Acknowledgement {
		callbackCount++
		return ibcmock.MockAcknowledgement
	}

	// send sequence 1 from chainA
	sequence1, err := path.EndpointA.SendPacket(defaultTimeoutHeight, disabledTimeoutTimestamp, ibctesting.MockPacketData)
	s.Require().NoError(err)
	packet1 := types.NewPacket(ibctesting.MockPacketData, sequence1,
		path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
		defaultTimeoutHeight, disabledTimeoutTimestamp)

	// receive sequence 1 for the first time: callback must fire once
	s.Require().NoError(path.EndpointB.RecvPacket(packet1))
	s.Require().Equal(uint64(1), callbackCount, "callback count after first receive of seq 1")

	// sync the light client so the duplicate receive can be verified
	s.Require().NoError(path.EndpointB.UpdateClient())

	// receive the SAME packet again: duplicate must NOT trigger the callback
	s.Require().NoError(path.EndpointB.RecvPacket(packet1))
	s.Require().Equal(uint64(1), callbackCount, "callback must not fire again for duplicate packet")

	// send a new sequence (2) from chainA
	sequence2, err := path.EndpointA.SendPacket(defaultTimeoutHeight, disabledTimeoutTimestamp, ibctesting.MockPacketData)
	s.Require().NoError(err)
	s.Require().Equal(sequence1+1, sequence2, "send sequence must be consecutive")
	packet2 := types.NewPacket(ibctesting.MockPacketData, sequence2,
		path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
		defaultTimeoutHeight, disabledTimeoutTimestamp)

	// sync the light client so the new sequence can be verified
	s.Require().NoError(path.EndpointB.UpdateClient())

	// receive sequence 2: callback must fire once more (twice total)
	s.Require().NoError(path.EndpointB.RecvPacket(packet2))
	s.Require().Equal(uint64(2), callbackCount, "callback count after receive of seq 2")

	// sync the light client so the keeper direct call can be verified
	s.Require().NoError(path.EndpointB.UpdateClient())

	// check state: receipts are stored per sequence and are not shared
	nextSeqRecv, found := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetNextSequenceRecv(s.chainB.GetContext(), packet1.GetDestPort(), packet1.GetDestChannel())
	s.Require().True(found)
	s.Require().Equal(uint64(1), nextSeqRecv, "next sequence recv must not change for unordered channel")

	receipt1, receipt1Stored := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceipt(s.chainB.GetContext(), packet1.GetDestPort(), packet1.GetDestChannel(), packet1.GetSequence())
	s.Require().True(receipt1Stored, "receipt for seq 1 must be stored")
	s.Require().Equal(string([]byte{byte(1)}), receipt1, "receipt for seq 1 must be the single byte marker")

	receipt2, receipt2Stored := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceipt(s.chainB.GetContext(), packet2.GetDestPort(), packet2.GetDestChannel(), packet2.GetSequence())
	s.Require().True(receipt2Stored, "receipt for seq 2 must be stored")
	s.Require().Equal(string([]byte{byte(1)}), receipt2, "receipt for seq 2 must be the single byte marker")

	// different sequences must not share receipts: the receipt for seq 1 must
	// not imply a receipt for seq 2 or any other sequence.
	_, receipt3Stored := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceipt(s.chainB.GetContext(), packet1.GetDestPort(), packet1.GetDestChannel(), sequence2+1)
	s.Require().False(receipt3Stored, "receipts must not be shared across sequences")

	// the receipt keys for the two sequences are distinct
	s.Require().NotEqual(
		host.PacketReceiptKey(packet1.GetDestPort(), packet1.GetDestChannel(), packet1.GetSequence()),
		host.PacketReceiptKey(packet2.GetDestPort(), packet2.GetDestChannel(), packet2.GetSequence()),
	)

	// query the proof of the (still committed) packet commitment on chainA so
	// the keeper can verify the duplicate receive directly.
	packetKey := host.PacketCommitmentKey(packet1.GetSourcePort(), packet1.GetSourceChannel(), packet1.GetSequence())
	proof, proofHeight := path.EndpointA.QueryProof(packetKey)

	// call the keeper directly on the duplicate path: it must return
	// ErrNoOpMsg together with a read-only receipt summary of the prior
	// processing result.
	channelVersion, summary, err := s.chainB.App.GetIBCKeeper().ChannelKeeper.RecvPacket(s.chainB.GetContext(), packet1, proof, proofHeight)
	s.Require().Error(err)
	s.Require().ErrorIs(err, types.ErrNoOpMsg)
	s.Require().Empty(channelVersion)
	s.Require().NotNil(summary, "duplicate path must return a receipt summary")

	// the summary reflects the prior processing result for seq 1
	s.Require().Equal(packet1.GetSequence(), summary.Sequence)
	s.Require().Equal(packet1.GetSourcePort(), summary.SourcePort)
	s.Require().Equal(packet1.GetSourceChannel(), summary.SourceChannel)
	s.Require().Equal(packet1.GetDestPort(), summary.DestPort)
	s.Require().Equal(packet1.GetDestChannel(), summary.DestChannel)
	s.Require().True(summary.AlreadyReceived, "summary must indicate the packet was already received")
	s.Require().Equal([]byte{byte(1)}, summary.Receipt, "summary must contain the stored receipt")
	s.Require().NotEmpty(summary.AcknowledgementCommitment, "summary must contain the ack commitment written by the prior processing")
	s.Require().Equal(types.CommitAcknowledgement(ibcmock.MockAcknowledgement.Acknowledgement()), summary.AcknowledgementCommitment)

	// the summary is read-only: building it must not mutate state
	receipt1After, _ := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceipt(s.chainB.GetContext(), packet1.GetDestPort(), packet1.GetDestChannel(), packet1.GetSequence())
	s.Require().Equal(receipt1, receipt1After)
	s.Require().Equal(uint64(2), callbackCount, "read-only summary must not trigger the callback")

	// a not-yet-received sequence must report as not received
	notReceived := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), types.NewPacket(
		ibctesting.MockPacketData, sequence2+1,
		path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
		defaultTimeoutHeight, disabledTimeoutTimestamp,
	))
	s.Require().Equal(sequence2+1, notReceived.Sequence)
	s.Require().False(notReceived.AlreadyReceived, "unknown sequence must report not received")
	s.Require().Empty(notReceived.Receipt)
	s.Require().Empty(notReceived.AcknowledgementCommitment)

	// the summary for seq 2 is distinct from the summary for seq 1
	summary2 := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), packet2)
	s.Require().Equal(packet2.GetSequence(), summary2.Sequence)
	s.Require().True(summary2.AlreadyReceived)
	s.Require().NotEqual(summary.Sequence, summary2.Sequence)
}

// TestRecvPacketDuplicateReceiptSummaryOrdered tests the receipt summary on the
// duplicate packet receive path for an ORDERED channel. The application
// callback is only invoked once per distinct sequence.
func (s *KeeperTestSuite) TestRecvPacketDuplicateReceiptSummaryOrdered() {
	var (
		path          *ibctesting.Path
		callbackCount uint64
	)

	path = ibctesting.NewPath(s.chainA, s.chainB)
	path.SetChannelOrdered()
	path.Setup()

	s.chainB.GetSimApp().IBCMockModule.IBCApp.OnRecvPacket = func(
		ctx sdk.Context, channelVersion string, packet types.Packet, relayer sdk.AccAddress,
	) exported.Acknowledgement {
		callbackCount++
		return ibcmock.MockAcknowledgement
	}

	// send sequence 1 and receive it
	sequence1, err := path.EndpointA.SendPacket(defaultTimeoutHeight, disabledTimeoutTimestamp, ibctesting.MockPacketData)
	s.Require().NoError(err)
	packet1 := types.NewPacket(ibctesting.MockPacketData, sequence1,
		path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
		defaultTimeoutHeight, disabledTimeoutTimestamp)

	s.Require().NoError(path.EndpointB.RecvPacket(packet1))
	s.Require().Equal(uint64(1), callbackCount)

	// sync the light client so the duplicate receive can be verified
	s.Require().NoError(path.EndpointB.UpdateClient())

	// receive the same packet again: duplicate must not trigger the callback
	s.Require().NoError(path.EndpointB.RecvPacket(packet1))
	s.Require().Equal(uint64(1), callbackCount)

	// the next receive sequence has been incremented for the ordered channel
	nextSeqRecv, found := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetNextSequenceRecv(s.chainB.GetContext(), packet1.GetDestPort(), packet1.GetDestChannel())
	s.Require().True(found)
	s.Require().Equal(uint64(2), nextSeqRecv)

	// sync the light client so the keeper direct call can be verified
	s.Require().NoError(path.EndpointB.UpdateClient())

	// query the proof and call the keeper directly on the duplicate path
	packetKey := host.PacketCommitmentKey(packet1.GetSourcePort(), packet1.GetSourceChannel(), packet1.GetSequence())
	proof, proofHeight := path.EndpointA.QueryProof(packetKey)

	channelVersion, summary, err := s.chainB.App.GetIBCKeeper().ChannelKeeper.RecvPacket(s.chainB.GetContext(), packet1, proof, proofHeight)
	s.Require().Error(err)
	s.Require().ErrorIs(err, types.ErrNoOpMsg)
	s.Require().Empty(channelVersion)
	s.Require().NotNil(summary)

	// on ordered channels the receipt is not stored; the next receive sequence
	// determines the prior processing result.
	s.Require().Equal(packet1.GetSequence(), summary.Sequence)
	s.Require().True(summary.AlreadyReceived, "summary must indicate the packet was already received")
	s.Require().Equal(uint64(2), summary.NextSequenceRecv, "summary must contain the next receive sequence")
	s.Require().Empty(summary.Receipt, "ordered channels do not store receipts")
	s.Require().NotEmpty(summary.AcknowledgementCommitment, "summary must contain the ack commitment written by the prior processing")
	s.Require().Equal(uint64(1), callbackCount, "callback must not fire again for duplicate packet")

	// send and receive a new sequence: callback fires once more (twice total)
	sequence2, err := path.EndpointA.SendPacket(defaultTimeoutHeight, disabledTimeoutTimestamp, ibctesting.MockPacketData)
	s.Require().NoError(err)
	packet2 := types.NewPacket(ibctesting.MockPacketData, sequence2,
		path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
		path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
		defaultTimeoutHeight, disabledTimeoutTimestamp)

	// sync the light client so the new sequence can be verified
	s.Require().NoError(path.EndpointB.UpdateClient())

	s.Require().NoError(path.EndpointB.RecvPacket(packet2))
	s.Require().Equal(uint64(2), callbackCount, "callback count after receive of seq 2")

	nextSeqRecv, found = s.chainB.App.GetIBCKeeper().ChannelKeeper.GetNextSequenceRecv(s.chainB.GetContext(), packet2.GetDestPort(), packet2.GetDestChannel())
	s.Require().True(found)
	s.Require().Equal(uint64(3), nextSeqRecv, "next sequence recv must be incremented for ordered channel")

	// the summary for seq 2 is distinct and reflects the new next receive sequence
	summary2 := s.chainB.App.GetIBCKeeper().ChannelKeeper.GetPacketReceiptSummary(s.chainB.GetContext(), packet2)
	s.Require().Equal(packet2.GetSequence(), summary2.Sequence)
	s.Require().True(summary2.AlreadyReceived)
	s.Require().Equal(uint64(3), summary2.NextSequenceRecv)
	s.Require().NotEqual(summary.Sequence, summary2.Sequence)
}

// TestRecvPacketReceiptSummaryString ensures the summary stringer is informative.
func (s *KeeperTestSuite) TestRecvPacketReceiptSummaryString() {
	summary := types.PacketReceiptSummary{
		Sequence:                  7,
		SourcePort:                "srcPort",
		SourceChannel:             "srcChannel",
		DestPort:                  "dstPort",
		DestChannel:               "dstChannel",
		AlreadyReceived:           true,
		NextSequenceRecv:          8,
		Receipt:                   []byte{1},
		AcknowledgementCommitment: []byte{0xaa, 0xbb},
	}
	str := summary.String()
	for _, expected := range []string{
		"sequence: 7",
		"src port: srcPort",
		"dst channel: dstChannel",
		"already received: true",
		"next sequence recv: 8",
		fmt.Sprintf("%X", []byte{1}),
		fmt.Sprintf("%X", []byte{0xaa, 0xbb}),
	} {
		s.Require().Contains(str, expected)
	}
}
