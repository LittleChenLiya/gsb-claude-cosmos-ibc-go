// SPDX-License-Identifier: Apache-2.0

package keeper_test

import (
	"encoding/hex"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
	"github.com/cosmos/ibc-go/v11/modules/core/03-connection/types"
	commitmenttypes "github.com/cosmos/ibc-go/v11/modules/core/23-commitment/types"
	host "github.com/cosmos/ibc-go/v11/modules/core/24-host"
	ibctm "github.com/cosmos/ibc-go/v11/modules/light-clients/07-tendermint"
	ibctesting "github.com/cosmos/ibc-go/v11/testing"
)

// TestVerifyConnectionStateDiagnostics verifies that connection proof
// verification failures are categorized into structured failure categories
// (prefix, path, root hash mismatch) and that failures do not modify the
// connection state. The test uses fixed proof fixtures generated against a
// single connection: a valid proof, a proof verified with the wrong
// counterparty prefix, and a proof verified against a modified consensus
// state root.
func (s *KeeperTestSuite) TestVerifyConnectionStateDiagnostics() {
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupConnections()

	connection := path.EndpointA.GetConnection()
	expectedConnection := path.EndpointB.GetConnection()
	connectionKey := host.ConnectionKey(path.EndpointB.ConnectionID)

	// Capture the connection state before any verification so we can confirm
	// that failed verification does not modify it.
	storedConnectionBefore := path.EndpointA.GetConnection()

	// Fixed proof fixture: a valid proof generated against the connection on
	// chainB.
	proof, proofHeight := s.chainB.QueryProof(connectionKey)

	// Case 1: valid proof. Verification should succeed.
	err := s.chainA.App.GetIBCKeeper().ConnectionKeeper.VerifyConnectionState(
		s.chainA.GetContext(), connection,
		proofHeight, proof, path.EndpointB.ConnectionID, expectedConnection,
	)
	s.Require().NoError(err)

	// Case 2: wrong prefix. The counterparty prefix on the connection does not
	// match the prefix the proof was generated against.
	wrongPrefixConnection := connection
	wrongPrefixConnection.Counterparty.Prefix = commitmenttypes.NewMerklePrefix([]byte("ibc2"))
	err = s.chainA.App.GetIBCKeeper().ConnectionKeeper.VerifyConnectionState(
		s.chainA.GetContext(), wrongPrefixConnection,
		proofHeight, proof, path.EndpointB.ConnectionID, expectedConnection,
	)
	s.Require().Error(err)
	s.Require().Equal(types.ProofVerificationCategoryPrefixMismatch, types.CategoryFromError(err))
	// The error text must not include the raw proof bytes.
	s.Require().NotContains(err.Error(), hex.EncodeToString(proof))

	// Case 3: wrong path. The connection identifier does not match the path the
	// proof was generated against.
	err = s.chainA.App.GetIBCKeeper().ConnectionKeeper.VerifyConnectionState(
		s.chainA.GetContext(), connection,
		proofHeight, proof, "connection-999", expectedConnection,
	)
	s.Require().Error(err)
	s.Require().Equal(types.ProofVerificationCategoryPathMismatch, types.CategoryFromError(err))
	s.Require().NotContains(err.Error(), hex.EncodeToString(proof))

	// Case 4: wrong root. Modify the consensus state root on chainA so that the
	// proof (generated against the original root) no longer commits to the
	// stored root.
	consensusState, found := s.chainA.GetConsensusState(path.EndpointA.ClientID, proofHeight)
	s.Require().True(found)
	tmConsensusState, ok := consensusState.(*ibctm.ConsensusState)
	s.Require().True(ok)
	modifiedRoot := make([]byte, len(tmConsensusState.Root.GetHash()))
	copy(modifiedRoot, tmConsensusState.Root.GetHash())
	modifiedRoot[0] ^= 0xff // flip the first byte to produce a different root hash
	tmConsensusState.Root = commitmenttypes.NewMerkleRoot(modifiedRoot)
	path.EndpointA.SetConsensusState(tmConsensusState, proofHeight)

	err = s.chainA.App.GetIBCKeeper().ConnectionKeeper.VerifyConnectionState(
		s.chainA.GetContext(), connection,
		proofHeight, proof, path.EndpointB.ConnectionID, expectedConnection,
	)
	s.Require().Error(err)
	s.Require().Equal(types.ProofVerificationCategoryRootMismatch, types.CategoryFromError(err))
	s.Require().NotContains(err.Error(), hex.EncodeToString(proof))

	// Confirm that failed verification did not modify the connection state.
	storedConnectionAfter := path.EndpointA.GetConnection()
	s.Require().Equal(storedConnectionBefore, storedConnectionAfter)
}

// TestVerifyConnectionStateDiagnosticsHeightMismatch verifies that a proof
// generated at a height greater than the client's latest height is categorized
// as a height mismatch.
func (s *KeeperTestSuite) TestVerifyConnectionStateDiagnosticsHeightMismatch() {
	path := ibctesting.NewPath(s.chainA, s.chainB)
	path.SetupConnections()

	connection := path.EndpointA.GetConnection()
	expectedConnection := path.EndpointB.GetConnection()
	connectionKey := host.ConnectionKey(path.EndpointB.ConnectionID)

	proof, proofHeight := s.chainB.QueryProof(connectionKey)

	// Use a proof height greater than the client's latest height.
	invalidHeight := clienttypes.NewHeight(proofHeight.GetRevisionNumber(), proofHeight.GetRevisionHeight()+100)
	err := s.chainA.App.GetIBCKeeper().ConnectionKeeper.VerifyConnectionState(
		s.chainA.GetContext(), connection,
		invalidHeight, proof, path.EndpointB.ConnectionID, expectedConnection,
	)
	s.Require().Error(err)
	s.Require().Equal(types.ProofVerificationCategoryHeightMismatch, types.CategoryFromError(err))
	s.Require().NotContains(err.Error(), hex.EncodeToString(proof))
}
