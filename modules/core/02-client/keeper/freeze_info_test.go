// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	"time"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
	"github.com/cosmos/ibc-go/v11/modules/core/exported"
	ibctm "github.com/cosmos/ibc-go/v11/modules/light-clients/07-tendermint"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
)

// tmHeaderEvidenceType is the registered protobuf message name of the 07-tendermint Header evidence.
const tmHeaderEvidenceType = "ibc.lightclients.tendermint.v1.Header"

// tmMisbehaviourEvidenceType is the registered protobuf message name of the 07-tendermint Misbehaviour evidence.
const tmMisbehaviourEvidenceType = "ibc.lightclients.tendermint.v1.Misbehaviour"

// createPastUpdateFn creates a valid 07-tendermint header which fills in a past
// height using the given trusted height.
func (s *KeeperTestSuite) createPastUpdateFn(path *ibctesting.Path, fillHeight, trustedHeight clienttypes.Height) *ibctm.Header {
	consState, found := s.chainA.App.GetIBCKeeper().ClientKeeper.GetClientConsensusState(s.chainA.GetContext(), path.EndpointA.ClientID, trustedHeight)
	s.Require().True(found)

	return s.chainB.CreateTMClientHeader(
		s.chainB.ChainID,
		int64(fillHeight.RevisionHeight),
		trustedHeight,
		consState.(*ibctm.ConsensusState).Timestamp.Add(time.Second*5),
		s.chainB.Vals, s.chainB.Vals, s.chainB.Vals, s.chainB.Signers,
	)
}

// TestGetClientFreezeInfo_EmptyForActiveClient verifies that an unfrozen client
// always returns an empty freeze explanation, both before and after a normal update.
func (s *KeeperTestSuite) TestGetClientFreezeInfo_EmptyForActiveClient() {
	s.SetupTest()
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupClients()

	clientKeeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()
	clientID := path.EndpointA.ClientID

	// before any freeze: empty explanation
	freezeInfo := clientKeeper.GetClientFreezeInfo(ctx, clientID)
	s.Require().True(freezeInfo.IsEmpty(), "expected empty freeze info for active client, got %s", freezeInfo.String())
	s.Require().Empty(freezeInfo.String())

	// perform a normal, non-conflicting update
	err := path.EndpointA.UpdateClient()
	s.Require().NoError(err)

	s.Require().Equal(exported.Active, clientKeeper.GetClientStatus(ctx, clientID))

	freezeInfo = clientKeeper.GetClientFreezeInfo(ctx, clientID)
	s.Require().True(freezeInfo.IsEmpty(), "expected empty freeze info after normal update, got %s", freezeInfo.String())
	s.Require().Empty(freezeInfo.String())
}

// TestGetClientFreezeInfo_ConflictingHeader verifies that a conflicting header
// update freezes the client and records the frozen height, the conflicting
// height and the triggering evidence type.
func (s *KeeperTestSuite) TestGetClientFreezeInfo_ConflictingHeader() {
	s.SetupTest()
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupClients()

	clientKeeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()
	clientID := path.EndpointA.ClientID

	height1 := clienttypes.NewHeight(1, 1)
	prevConsState := &ibctm.ConsensusState{
		Timestamp:          s.past,
		NextValidatorsHash: s.chainB.Vals.Hash(),
	}
	clientKeeper.SetClientConsensusState(ctx, clientID, height1, prevConsState)

	height5 := clienttypes.NewHeight(1, 5)
	nextConsState := &ibctm.ConsensusState{
		Timestamp:          s.past.Add(time.Minute),
		NextValidatorsHash: s.chainB.Vals.Hash(),
	}
	clientKeeper.SetClientConsensusState(ctx, clientID, height5, nextConsState)

	height3 := clienttypes.NewHeight(1, 3)
	updateHeader := s.createPastUpdateFn(path, height3, height1)

	// store a conflicting consensus state at the same height
	conflictConsState := updateHeader.ConsensusState()
	conflictConsState.Root = commitmenttypes.NewMerkleRoot([]byte("conflicting apphash"))
	clientKeeper.SetClientConsensusState(ctx, clientID, updateHeader.GetHeight(), conflictConsState)

	// submitting the conflicting header freezes the client; no error is returned
	err := clientKeeper.UpdateClient(ctx, clientID, updateHeader)
	s.Require().NoError(err)

	s.Require().Equal(exported.Frozen, clientKeeper.GetClientStatus(ctx, clientID))

	frozenClientState, found := clientKeeper.GetClientState(ctx, clientID)
	s.Require().True(found)
	s.Require().False(frozenClientState.(*ibctm.ClientState).FrozenHeight.IsZero())

	freezeInfo := clientKeeper.GetClientFreezeInfo(ctx, clientID)
	s.Require().False(freezeInfo.IsEmpty(), "expected non-empty freeze info for frozen client")
	s.Require().Equal(ibctm.FrozenHeight, freezeInfo.FrozenHeight, "frozen height should match the client state frozen height")
	s.Require().Equal(tmHeaderEvidenceType, freezeInfo.EvidenceType)
	s.Require().Equal([]clienttypes.Height{height3}, freezeInfo.ConflictingHeights)
	s.Require().NotEmpty(freezeInfo.String())
	s.T().Logf("freeze explanation (conflicting header): %s", freezeInfo.String())
}

// TestGetClientFreezeInfo_SubmittedMisbehaviour verifies that a submitted
// Misbehaviour client message freezes the client and records both conflicting
// header heights and the misbehaviour evidence type.
func (s *KeeperTestSuite) TestGetClientFreezeInfo_SubmittedMisbehaviour() {
	s.SetupTest()
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupClients()

	clientKeeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()
	clientID := path.EndpointA.ClientID

	trustedHeight, ok := path.EndpointA.GetClientLatestHeight().(clienttypes.Height)
	s.Require().True(ok)

	trustedVals, ok := s.chainB.TrustedValidators[trustedHeight.RevisionHeight]
	s.Require().True(ok)

	// advance the client once so the conflicting height is a past height of the client
	err := path.EndpointA.UpdateClient()
	s.Require().NoError(err)
	latestHeight, ok := path.EndpointA.GetClientLatestHeight().(clienttypes.Height)
	s.Require().True(ok)

	// fork misbehaviour: two different headers at the same height
	misbehaviour := &ibctm.Misbehaviour{
		Header1: s.chainB.CreateTMClientHeader(s.chainB.ChainID, int64(latestHeight.RevisionHeight), trustedHeight, s.chainB.ProposedHeader.Time.Add(time.Minute), s.chainB.Vals, s.chainB.NextVals, trustedVals, s.chainB.Signers),
		Header2: s.chainB.CreateTMClientHeader(s.chainB.ChainID, int64(latestHeight.RevisionHeight), trustedHeight, s.chainB.ProposedHeader.Time, s.chainB.Vals, s.chainB.NextVals, trustedVals, s.chainB.Signers),
	}

	err = clientKeeper.UpdateClient(ctx, clientID, misbehaviour)
	s.Require().NoError(err)

	s.Require().Equal(exported.Frozen, clientKeeper.GetClientStatus(ctx, clientID))

	freezeInfo := clientKeeper.GetClientFreezeInfo(ctx, clientID)
	s.Require().False(freezeInfo.IsEmpty(), "expected non-empty freeze info for frozen client")
	s.Require().Equal(ibctm.FrozenHeight, freezeInfo.FrozenHeight)
	s.Require().Equal(tmMisbehaviourEvidenceType, freezeInfo.EvidenceType)
	s.Require().Equal([]clienttypes.Height{latestHeight, latestHeight}, freezeInfo.ConflictingHeights)
	s.Require().NotEmpty(freezeInfo.String())
	s.T().Logf("freeze explanation (submitted misbehaviour): %s", freezeInfo.String())
}

// TestGetClientFreezeInfo_FrozenWithoutRecord verifies the compatibility path
// for a client which is frozen but has no recorded freeze explanation (e.g.
// state migrated from before the freeze-info feature): the explanation is empty
// while the status remains Frozen.
func (s *KeeperTestSuite) TestGetClientFreezeInfo_FrozenWithoutRecord() {
	s.SetupTest()
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupClients()

	clientKeeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()
	clientID := path.EndpointA.ClientID

	// manually freeze the client state without writing the freeze-info record
	clientState, ok := path.EndpointA.GetClientState().(*ibctm.ClientState)
	s.Require().True(ok)
	clientState.FrozenHeight = clienttypes.NewHeight(1, 1)
	clientKeeper.SetClientState(ctx, clientID, clientState)

	s.Require().Equal(exported.Frozen, clientKeeper.GetClientStatus(ctx, clientID))

	freezeInfo := clientKeeper.GetClientFreezeInfo(ctx, clientID)
	s.Require().True(freezeInfo.IsEmpty(), "expected empty freeze info for legacy frozen client, got %s", freezeInfo.String())
	s.Require().Empty(freezeInfo.String())
}

// TestGetClientFreezeInfo_ClearedOnRecovery verifies that recovering a frozen
// client removes the stale freeze explanation, so that the recovered (active)
// client returns an empty explanation.
func (s *KeeperTestSuite) TestGetClientFreezeInfo_ClearedOnRecovery() {
	s.SetupTest()

	subjectPath := ibctesting.NewPath(s.chainA, s.chainB)
	subjectPath.SetupClients()
	subject := subjectPath.EndpointA.ClientID

	substitutePath := ibctesting.NewPath(s.chainA, s.chainB)
	substitutePath.SetupClients()
	substitute := substitutePath.EndpointA.ClientID

	// update substitute twice so its latest height is ahead of the subject
	s.Require().NoError(substitutePath.EndpointA.UpdateClient())
	s.Require().NoError(substitutePath.EndpointA.UpdateClient())

	clientKeeper := s.chainA.App.GetIBCKeeper().ClientKeeper
	ctx := s.chainA.GetContext()

	// freeze the subject with a conflicting header and assert the explanation exists
	height1 := clienttypes.NewHeight(1, 1)
	prevConsState := &ibctm.ConsensusState{
		Timestamp:          s.past,
		NextValidatorsHash: s.chainB.Vals.Hash(),
	}
	clientKeeper.SetClientConsensusState(ctx, subject, height1, prevConsState)

	height5 := clienttypes.NewHeight(1, 5)
	nextConsState := &ibctm.ConsensusState{
		Timestamp:          s.past.Add(time.Minute),
		NextValidatorsHash: s.chainB.Vals.Hash(),
	}
	clientKeeper.SetClientConsensusState(ctx, subject, height5, nextConsState)

	height3 := clienttypes.NewHeight(1, 3)
	updateHeader := s.createPastUpdateFn(subjectPath, height3, height1)
	conflictConsState := updateHeader.ConsensusState()
	conflictConsState.Root = commitmenttypes.NewMerkleRoot([]byte("conflicting apphash"))
	clientKeeper.SetClientConsensusState(ctx, subject, updateHeader.GetHeight(), conflictConsState)

	s.Require().NoError(clientKeeper.UpdateClient(ctx, subject, updateHeader))
	s.Require().Equal(exported.Frozen, clientKeeper.GetClientStatus(ctx, subject))
	s.Require().False(clientKeeper.GetClientFreezeInfo(ctx, subject).IsEmpty())

	// recover the subject from the active substitute
	s.Require().NoError(clientKeeper.RecoverClient(ctx, subject, substitute))

	s.Require().Equal(exported.Active, clientKeeper.GetClientStatus(ctx, subject))
	s.Require().True(clientKeeper.GetClientFreezeInfo(ctx, subject).IsEmpty(),
		"expected stale freeze info to be cleared after recovery")
}
