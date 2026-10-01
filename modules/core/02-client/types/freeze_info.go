// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"
)

// FreezeInfo is a read-only record describing why a light client was frozen.
// It is written to the client-specific store when a verified misbehaviour
// causes the client to be frozen during UpdateClient.
//
// The zero value represents an unfrozen client: an empty FreezeInfo is always
// returned for clients which are not frozen.
type FreezeInfo struct {
	// FrozenHeight is the height at which the client state was frozen.
	FrozenHeight Height `json:"frozen_height,omitempty"`
	// ConflictingHeights are the heights involved in the detected misbehaviour.
	// A conflicting header update reports the header height; a submitted
	// misbehaviour reports the heights of its conflicting headers.
	ConflictingHeights []Height `json:"conflicting_heights,omitempty"`
	// EvidenceType is the protobuf message type name of the ClientMessage
	// that triggered the freeze (e.g. Header or Misbehaviour).
	EvidenceType string `json:"evidence_type,omitempty"`
}

// IsEmpty returns true if no freeze information is recorded.
func (fi FreezeInfo) IsEmpty() bool {
	return fi.FrozenHeight.IsZero() &&
		len(fi.ConflictingHeights) == 0 &&
		strings.TrimSpace(fi.EvidenceType) == ""
}

// String returns a human readable description of the freeze information.
// It returns an empty string when no freeze information is recorded.
func (fi FreezeInfo) String() string {
	if fi.IsEmpty() {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("client frozen")
	if !fi.FrozenHeight.IsZero() {
		sb.WriteString(" at height ")
		sb.WriteString(fi.FrozenHeight.String())
	}
	if strings.TrimSpace(fi.EvidenceType) != "" {
		sb.WriteString(" by evidence type ")
		sb.WriteString(fi.EvidenceType)
	}
	if len(fi.ConflictingHeights) > 0 {
		sb.WriteString(" at conflicting height(s)")
		for _, h := range fi.ConflictingHeights {
			sb.WriteString(" ")
			sb.WriteString(h.String())
		}
	}

	return sb.String()
}
