// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v11/modules/core/24-host"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
)

func containsTimeoutEvent(events []abci.Event, packet types.Packet) bool {
	for _, event := range events {
		if event.Type == types.EventTypeTimeoutPacket {
			for _, attr := range event.Attributes {
				if attr.Key == types.AttributeKeySequence && attr.Value == fmt.Sprintf("%d", packet.GetSequence()) {
					return true
				}
			}
		}
	}
	return false
}

func (s *KeeperTestSuite) TestTimeoutPacketWithReason() {
	var (
		path        *ibctesting.Path
		packet      types.Packet
		nextSeqRecv uint64
		ctx         sdk.Context
	)

	testCases := []struct {
		msg      string
		malleate func()
		proof    func() ([]byte, exported.Height)
		exp      func(reason types.TimeoutReason, err error)
	}{
		{
			"height timeout: reason is HEIGHT_TIMEOUT, commitment deleted, event emitted",
			func() {
				path.Setup()

				timeoutHeight := clienttypes.GetSelfHeight(s.chainB.GetContext())
				timeoutTimestamp := disabledTimeoutTimestamp

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, timeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)
				packet = types.NewPacket(ibctesting.MockPacketData, sequence, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, timeoutHeight, timeoutTimestamp)
				err = path.EndpointA.UpdateClient()
				s.Require().NoError(err)
			},
			func() ([]byte, exported.Height) {
				unorderedPacketKey := host.PacketReceiptKey(packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
				return path.EndpointB.QueryProof(unorderedPacketKey)
			},
			func(reason types.TimeoutReason, err error) {
				s.Require().NoError(err)
				s.Require().Equal(types.HEIGHT_TIMEOUT, reason)

				commitment := s.chainA.App.GetIBCKeeper().ChannelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().Empty(commitment)

				events := ctx.EventManager().ABCIEvents()
				s.Require().True(containsTimeoutEvent(events, packet), "timeout event should be emitted")
			},
		},
		{
			"timestamp timeout: reason is TIMESTAMP_TIMEOUT, commitment deleted, event emitted",
			func() {
				path.Setup()

				timeoutHeight := disabledTimeoutHeight
				timeoutTimestamp := uint64(s.chainB.GetContext().BlockTime().UnixNano())

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, timeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)
				packet = types.NewPacket(ibctesting.MockPacketData, sequence, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, timeoutHeight, timeoutTimestamp)
				err = path.EndpointA.UpdateClient()
				s.Require().NoError(err)
			},
			func() ([]byte, exported.Height) {
				unorderedPacketKey := host.PacketReceiptKey(packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
				return path.EndpointB.QueryProof(unorderedPacketKey)
			},
			func(reason types.TimeoutReason, err error) {
				s.Require().NoError(err)
				s.Require().Equal(types.TIMESTAMP_TIMEOUT, reason)

				commitment := s.chainA.App.GetIBCKeeper().ChannelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().Empty(commitment)

				events := ctx.EventManager().ABCIEvents()
				s.Require().True(containsTimeoutEvent(events, packet), "timeout event should be emitted")
			},
		},
		{
			"invalid proof: reason is INVALID_PROOF, commitment retained, no event",
			func() {
				path.Setup()

				timeoutHeight := clienttypes.GetSelfHeight(s.chainB.GetContext())
				timeoutTimestamp := disabledTimeoutTimestamp

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, timeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)
				packet = types.NewPacket(ibctesting.MockPacketData, sequence, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, timeoutHeight, timeoutTimestamp)
				err = path.EndpointA.UpdateClient()
				s.Require().NoError(err)
			},
			func() ([]byte, exported.Height) {
				proofHeight := path.EndpointA.GetClientLatestHeight()
				return []byte{}, proofHeight
			},
			func(reason types.TimeoutReason, err error) {
				s.Require().Error(err)
				s.Require().Equal(types.INVALID_PROOF, reason)

				commitment := s.chainA.App.GetIBCKeeper().ChannelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().NotEmpty(commitment)

				events := ctx.EventManager().ABCIEvents()
				s.Require().False(containsTimeoutEvent(events, packet), "timeout event should not be emitted on proof failure")
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.msg, func() {
			s.SetupTest()
			nextSeqRecv = 1
			path = ibctesting.NewPath(s.chainA, s.chainB)

			tc.malleate()

			proof, proofHeight := tc.proof()

			ctx = s.chainA.GetContext()
			_, reason, err := s.chainA.App.GetIBCKeeper().ChannelKeeper.TimeoutPacketWithReason(ctx, packet, proof, proofHeight, nextSeqRecv)

			tc.exp(reason, err)
		})
	}
}

func (s *KeeperTestSuite) TestTimeoutPacketCompatibility() {
	var (
		path        *ibctesting.Path
		packet      types.Packet
		nextSeqRecv uint64
		ctx         sdk.Context
	)

	testCases := []struct {
		msg      string
		malleate func()
		proof    func() ([]byte, exported.Height)
		expErr   bool
	}{
		{
			"success: height timeout returns no error",
			func() {
				path.Setup()

				timeoutHeight := clienttypes.GetSelfHeight(s.chainB.GetContext())
				timeoutTimestamp := disabledTimeoutTimestamp

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, timeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)
				packet = types.NewPacket(ibctesting.MockPacketData, sequence, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, timeoutHeight, timeoutTimestamp)
				err = path.EndpointA.UpdateClient()
				s.Require().NoError(err)
			},
			func() ([]byte, exported.Height) {
				unorderedPacketKey := host.PacketReceiptKey(packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
				return path.EndpointB.QueryProof(unorderedPacketKey)
			},
			false,
		},
		{
			"failure: invalid proof returns error and retains commitment",
			func() {
				path.Setup()

				timeoutHeight := clienttypes.GetSelfHeight(s.chainB.GetContext())
				timeoutTimestamp := disabledTimeoutTimestamp

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, timeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)
				packet = types.NewPacket(ibctesting.MockPacketData, sequence, path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, timeoutHeight, timeoutTimestamp)
				err = path.EndpointA.UpdateClient()
				s.Require().NoError(err)
			},
			func() ([]byte, exported.Height) {
				proofHeight := path.EndpointA.GetClientLatestHeight()
				return []byte{}, proofHeight
			},
			true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.msg, func() {
			s.SetupTest()
			nextSeqRecv = 1
			path = ibctesting.NewPath(s.chainA, s.chainB)

			tc.malleate()

			proof, proofHeight := tc.proof()

			ctx = s.chainA.GetContext()
			channelVersion, err := s.chainA.App.GetIBCKeeper().ChannelKeeper.TimeoutPacket(ctx, packet, proof, proofHeight, nextSeqRecv)

			if tc.expErr {
				s.Require().Error(err)
				s.Require().Empty(channelVersion)

				commitment := s.chainA.App.GetIBCKeeper().ChannelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().NotEmpty(commitment)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(path.EndpointA.GetChannel().Version, channelVersion)

				commitment := s.chainA.App.GetIBCKeeper().ChannelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().Empty(commitment)
			}
		})
	}
}
