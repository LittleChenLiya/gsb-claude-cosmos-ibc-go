// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	"time"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibctm "github.com/cosmos/ibc-go/v11/modules/light-clients/07-tendermint"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
)

// TestGetClientFrozenState verifies the read-only frozen state description for a
// light client. It constructs both a normal update and a conflict (misbehaviour)
// update and checks that:
//   - an unfrozen client returns an empty description,
//   - a normal update still succeeds (compatibility path is unchanged),
//   - a conflict update freezes the client at the sentinel frozen height {0,1},
//   - the frozen state description contains the conflict height and the triggering
//     evidence type,
//   - a subsequent normal update is rejected because the client is no longer active
//     (consensus check is unchanged).
func (s *KeeperTestSuite) TestGetClientFrozenState() {
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupClients()

	clientID := path.EndpointA.ClientID
	keeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()
	signer := s.chainA.SenderAccount.GetAddress().String()

	// an unfrozen client returns an empty description
	frozenState, frozen := keeper.GetClientFrozenState(ctx, clientID)
	s.Require().False(frozen)
	s.Require().Equal(exported.FrozenState{}, frozenState)

	// the client is active
	s.Require().Equal(exported.Active, keeper.GetClientStatus(ctx, clientID))

	// normal update succeeds (compatibility path is unchanged)
	s.Require().NoError(path.EndpointA.UpdateClient())
	s.Require().Equal(exported.Active, keeper.GetClientStatus(ctx, clientID))

	// construct a valid fork misbehaviour: two headers at the same height with
	// different timestamps (different commit hashes)
	trustedHeight, ok := path.EndpointA.GetClientLatestHeight().(clienttypes.Height)
	s.Require().True(ok)

	trustedVals, ok := s.chainB.TrustedValidators[trustedHeight.RevisionHeight]
	s.Require().True(ok)

	s.Require().NoError(path.EndpointA.UpdateClient())

	height, ok := path.EndpointA.GetClientLatestHeight().(clienttypes.Height)
	s.Require().True(ok)

	misbehaviour := &ibctm.Misbehaviour{
		Header1: s.chainB.CreateTMClientHeader(s.chainB.ChainID, int64(height.RevisionHeight), trustedHeight, s.chainB.ProposedHeader.Time.Add(time.Minute), s.chainB.Vals, s.chainB.NextVals, trustedVals, s.chainB.Signers),
		Header2: s.chainB.CreateTMClientHeader(s.chainB.ChainID, int64(height.RevisionHeight), trustedHeight, s.chainB.ProposedHeader.Time, s.chainB.Vals, s.chainB.NextVals, trustedVals, s.chainB.Signers),
	}
	misbehaviour.ClientId = clientID

	conflictHeight := misbehaviour.Header1.GetHeight()

	// submit the conflict update via a transaction; the client must freeze
	msg, err := clienttypes.NewMsgUpdateClient(clientID, misbehaviour, signer)
	s.Require().NoError(err)
	_, err = s.chainA.SendMsgs(msg)
	s.Require().NoError(err)
	s.Require().Equal(exported.Frozen, keeper.GetClientStatus(s.chainA.GetContext(), clientID))

	// the frozen height is the sentinel value {0,1}
	clientState, ok := keeper.GetClientState(s.chainA.GetContext(), clientID)
	s.Require().True(ok)
	tmClientState, ok := clientState.(*ibctm.ClientState)
	s.Require().True(ok)
	s.Require().Equal(ibctm.FrozenHeight, tmClientState.FrozenHeight)

	// the frozen state description is populated with the conflict height and evidence type
	frozenState, frozen = keeper.GetClientFrozenState(s.chainA.GetContext(), clientID)
	s.Require().True(frozen)
	s.Require().Equal(ibctm.FrozenHeight, frozenState.FrozenHeight)
	s.Require().Equal(conflictHeight, frozenState.ConflictHeight)
	s.Require().Equal("/ibc.lightclients.tendermint.v1.Misbehaviour", frozenState.EvidenceType)

	// a subsequent normal update is rejected because the client is frozen
	err = path.EndpointA.UpdateClient()
	s.Require().Error(err)
	s.Require().ErrorContains(err, "client state is not active")
}

// TestGetClientFrozenStateHeaderEvidence verifies that a client frozen via a
// Header message (duplicate height misbehaviour) records the Header evidence type
// and the header's height as the conflict height.
func (s *KeeperTestSuite) TestGetClientFrozenStateHeaderEvidence() {
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupClients()

	clientID := path.EndpointA.ClientID
	keeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()
	signer := s.chainA.SenderAccount.GetAddress().String()

	// construct a header at the counterparty's latest height, which is greater
	// than the client's initial latest height
	trustedHeight, ok := path.EndpointA.GetClientLatestHeight().(clienttypes.Height)
	s.Require().True(ok)

	header, err := path.EndpointA.Counterparty.Chain.IBCClientHeader(path.EndpointA.Counterparty.Chain.LatestCommittedHeader, trustedHeight)
	s.Require().NoError(err)

	// store a conflicting consensus state at the header's height to trigger misbehaviour
	conflictingConsState := &ibctm.ConsensusState{
		Timestamp:          header.GetTime().Add(time.Hour),
		Root:               header.ConsensusState().Root,
		NextValidatorsHash: header.Header.NextValidatorsHash,
	}
	keeper.SetClientConsensusState(ctx, clientID, header.GetHeight(), conflictingConsState)

	// submit the header via a transaction; the client must freeze due to the
	// duplicate height misbehaviour
	msg, err := clienttypes.NewMsgUpdateClient(clientID, header, signer)
	s.Require().NoError(err)
	_, err = s.chainA.SendMsgs(msg)
	s.Require().NoError(err)
	s.Require().Equal(exported.Frozen, keeper.GetClientStatus(s.chainA.GetContext(), clientID))

	// the frozen state description records the Header evidence type and the header's height
	frozenState, frozen := keeper.GetClientFrozenState(s.chainA.GetContext(), clientID)
	s.Require().True(frozen)
	s.Require().Equal(ibctm.FrozenHeight, frozenState.FrozenHeight)
	s.Require().Equal(header.GetHeight(), frozenState.ConflictHeight)
	s.Require().Equal("/ibc.lightclients.tendermint.v1.Header", frozenState.EvidenceType)
}
