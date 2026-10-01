// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	abci "github.com/cometbft/cometbft/abci/types"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/04-channel/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
	host "github.com/cosmos/ibc-go/v11/modules/core/24-host"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
)

// hasEventType reports whether the given event type is present in the events.
func hasEventType(events []abci.Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}

	return false
}

// queryTimeoutProof returns a valid timeout proof and proof height from the
// counterparty endpoint for the given packet, using the key dictated by the
// channel ordering.
func queryTimeoutProof(path *ibctesting.Path, ordered bool, packet types.Packet) ([]byte, clienttypes.Height) {
	var packetKey []byte
	if ordered {
		packetKey = host.NextSequenceRecvKey(packet.GetDestPort(), packet.GetDestChannel())
	} else {
		packetKey = host.PacketReceiptKey(packet.GetDestPort(), packet.GetDestChannel(), packet.GetSequence())
	}

	proof, proofHeight := path.EndpointB.QueryProof(packetKey)
	return proof, proofHeight
}

// TestTimeoutPacketReasonBreakdown exercises the timeout reason breakdown
// using the in-memory ibctesting keeper fixtures. It injects the three
// relevant situations independently and checks, for each, the queryable
// reason, whether the packet commitment is retained, and which events the
// write path (TimeoutPacket) emits:
//
//  1. height expiry:    proof valid -> commitment deleted, timeout event emitted;
//  2. timestamp expiry: proof valid -> commitment deleted, timeout event emitted;
//  3. invalid proof:    commitment retained, no timeout event, original
//     proof-verification error code returned.
func (s *KeeperTestSuite) TestTimeoutPacketReasonBreakdown() {
	type fixture struct {
		name      string
		setup     func(path *ibctesting.Path) (types.Packet, []byte, clienttypes.Height, uint64)
		expReason types.TimeoutReason
		expErr    error // nil on success, otherwise the sentinel error matched with errors.Is
	}

	fixtures := []fixture{
		{
			name: "height elapsed on unordered channel",
			setup: func(path *ibctesting.Path) (types.Packet, []byte, clienttypes.Height, uint64) {
				path.Setup()

				// timeout height is the counterparty's current height; no
				// timestamp timeout is configured.
				timeoutHeight := clienttypes.GetSelfHeight(s.chainB.GetContext())

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, disabledTimeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)

				packet := types.NewPacket(
					ibctesting.MockPacketData, sequence,
					path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
					path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
					timeoutHeight, disabledTimeoutTimestamp,
				)
				s.Require().NoError(path.EndpointA.UpdateClient())

				proof, proofHeight := queryTimeoutProof(path, false, packet)
				return packet, proof, proofHeight, 1
			},
			expReason: types.TimeoutReasonHeight,
			expErr:    nil,
		},
		{
			name: "timestamp elapsed on unordered channel",
			setup: func(path *ibctesting.Path) (types.Packet, []byte, clienttypes.Height, uint64) {
				path.Setup()

				// zero timeout height: only the timestamp can elapse.
				timeoutTimestamp := uint64(s.chainB.GetContext().BlockTime().UnixNano())

				sequence, err := path.EndpointA.SendPacket(clienttypes.ZeroHeight(), timeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)

				packet := types.NewPacket(
					ibctesting.MockPacketData, sequence,
					path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
					path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
					clienttypes.ZeroHeight(), timeoutTimestamp,
				)
				s.Require().NoError(path.EndpointA.UpdateClient())

				proof, proofHeight := queryTimeoutProof(path, false, packet)
				return packet, proof, proofHeight, 1
			},
			expReason: types.TimeoutReasonTimestamp,
			expErr:    nil,
		},
		{
			name: "height elapsed but proof invalid on ordered channel",
			setup: func(path *ibctesting.Path) (types.Packet, []byte, clienttypes.Height, uint64) {
				path.SetChannelOrdered()
				path.Setup()

				timeoutHeight := clienttypes.GetSelfHeight(s.chainB.GetContext())

				sequence, err := path.EndpointA.SendPacket(timeoutHeight, disabledTimeoutTimestamp, ibctesting.MockPacketData)
				s.Require().NoError(err)

				packet := types.NewPacket(
					ibctesting.MockPacketData, sequence,
					path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
					path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID,
					timeoutHeight, disabledTimeoutTimestamp,
				)
				s.Require().NoError(path.EndpointA.UpdateClient())

				// The channel is ORDERED, but request the proof of the
				// UNORDERED receipt-absence key instead of the next sequence
				// receive key, producing a deterministic invalid proof.
				proof, proofHeight := queryTimeoutProof(path, false, packet)
				return packet, proof, proofHeight, 1
			},
			expReason: types.TimeoutReasonProofInvalid,
			expErr:    commitmenttypes.ErrInvalidProof,
		},
	}

	for _, f := range fixtures {
		s.Run(f.name, func() {
			s.SetupTest() // reset in-memory keepers
			path := ibctesting.NewPath(s.chainA, s.chainB)

			packet, proof, proofHeight, nextSeqRecv := f.setup(path)

			ctx := s.chainA.GetContext()
			channelKeeper := s.chainA.App.GetIBCKeeper().ChannelKeeper

			// precondition: the packet commitment exists on the sender
			commitmentBefore := channelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
			s.Require().NotEmpty(commitmentBefore, "packet commitment must exist before timeout")

			// 1) queryable reason path: must be side-effect free
			reason, reasonErr := channelKeeper.PacketTimeoutReason(ctx, packet, proof, proofHeight, nextSeqRecv)
			s.Require().Equal(f.expReason, reason)
			if f.expErr == nil {
				s.Require().NoError(reasonErr)
			} else {
				s.Require().ErrorIs(reasonErr, f.expErr)
			}

			commitmentAfterQuery := channelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
			s.Require().Equal(commitmentBefore, commitmentAfterQuery, "PacketTimeoutReason must not mutate state")
			s.Require().False(
				hasEventType(ctx.EventManager().ABCIEvents(), types.EventTypeTimeoutPacket),
				"PacketTimeoutReason must not emit timeout events",
			)

			// 2) write path: TimeoutPacket with the same inputs, run in a
			// cached context so we can inspect (and, on success, commit) the
			// exact effects.
			writeCtx, writeFn := ctx.CacheContext()
			_, err := channelKeeper.TimeoutPacket(writeCtx, packet, proof, proofHeight, nextSeqRecv)

			if f.expErr == nil {
				s.Require().NoError(err)
				writeFn()

				// commitment deleted and timeout event emitted
				commitmentAfter := channelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().Empty(commitmentAfter, "commitment must be deleted after a successful timeout")

				s.Require().True(
					hasEventType(writeCtx.EventManager().ABCIEvents(), types.EventTypeTimeoutPacket),
					"successful timeout must emit the timeout_packet event",
				)
			} else {
				s.Require().ErrorIs(err, f.expErr)
				// writeFn is intentionally not called: the failed cache
				// context is discarded. The parent context is untouched.
				commitmentAfter := channelKeeper.GetPacketCommitment(ctx, packet.GetSourcePort(), packet.GetSourceChannel(), packet.GetSequence())
				s.Require().Equal(commitmentBefore, commitmentAfter, "commitment must be retained after a proof failure")
				s.Require().False(
					hasEventType(writeCtx.EventManager().ABCIEvents(), types.EventTypeTimeoutPacket),
					"proof failure must not emit the timeout_packet event",
				)
			}
		})
	}
}
