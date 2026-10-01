// SPDX-License-Identifier: Apache-2.0

package types

import (
	errorsmod "cosmossdk.io/errors"

	clienttypes "github.com/cosmos/ibc-go/v11/modules/core/02-client/types"
)

// TimeoutReason is the queryable, fine-grained reason why a packet timeout
// path can be exercised (or fail) on the sending chain. It allows relayers
// and query clients to distinguish between a timeout caused by the
// counterparty client reaching the timeout height, reaching the timeout
// timestamp, and a timeout proof that failed verification.
type TimeoutReason int32

const (
	// TimeoutReasonUnspecified is the zero value; it is returned when the
	// timeout has not elapsed (and therefore no reason applies).
	TimeoutReasonUnspecified TimeoutReason = iota
	// TimeoutReasonHeight indicates that the timeout height has been reached
	// on the counterparty client.
	TimeoutReasonHeight
	// TimeoutReasonTimestamp indicates that the timeout timestamp has been
	// reached on the counterparty client.
	TimeoutReasonTimestamp
	// TimeoutReasonProofInvalid indicates that, although the timeout has
	// elapsed, the proof that the packet has not been received (or, for
	// ORDERED channels, the proof of next sequence recv) failed
	// verification. The packet commitment must be retained in this case.
	TimeoutReasonProofInvalid
)

// String returns the canonical string representation of a TimeoutReason.
func (r TimeoutReason) String() string {
	switch r {
	case TimeoutReasonHeight:
		return "TIMEOUT_REASON_HEIGHT"
	case TimeoutReasonTimestamp:
		return "TIMEOUT_REASON_TIMESTAMP"
	case TimeoutReasonProofInvalid:
		return "TIMEOUT_REASON_PROOF_INVALID"
	default:
		return "TIMEOUT_REASON_UNSPECIFIED"
	}
}

// Valid returns true if the TimeoutReason is one of the defined, non-zero
// timeout reasons.
func (r TimeoutReason) Valid() bool {
	return r == TimeoutReasonHeight || r == TimeoutReasonTimestamp || r == TimeoutReasonProofInvalid
}

// NewTimeout returns a new Timeout instance.
func NewTimeout(height clienttypes.Height, timestamp uint64) Timeout {
	return Timeout{
		Height:    height,
		Timestamp: timestamp,
	}
}

// NewTimeoutWithTimestamp creates a new Timeout with only the timestamp set.
func NewTimeoutWithTimestamp(timestamp uint64) Timeout {
	return Timeout{
		Height:    clienttypes.ZeroHeight(),
		Timestamp: timestamp,
	}
}

// IsValid returns true if either the height or timestamp is non-zero.
func (t Timeout) IsValid() bool {
	return !t.Height.IsZero() || t.Timestamp != 0
}

// Elapsed returns true if either the provided height or timestamp is past the
// respective absolute timeout values.
func (t Timeout) Elapsed(height clienttypes.Height, timestamp uint64) bool {
	return t.heightElapsed(height) || t.timestampElapsed(timestamp)
}

// TimestampElapsed returns true if the provided timestamp is past the timeout timestamp.
func (t Timeout) TimestampElapsed(timestamp uint64) bool {
	return t.timestampElapsed(timestamp)
}

// Reason returns the fine-grained reason explaining why the timeout is
// considered elapsed against the provided counterparty height and timestamp.
// Height expiry takes precedence over timestamp expiry, mirroring the
// ordering used by ErrTimeoutElapsed. TimeoutReasonUnspecified is returned
// when neither the timeout height nor the timeout timestamp has elapsed.
func (t Timeout) Reason(height clienttypes.Height, timestamp uint64) TimeoutReason {
	switch {
	case t.heightElapsed(height):
		return TimeoutReasonHeight
	case t.timestampElapsed(timestamp):
		return TimeoutReasonTimestamp
	default:
		return TimeoutReasonUnspecified
	}
}

// ErrTimeoutElapsed returns a timeout elapsed error indicating which timeout value
// has elapsed.
func (t Timeout) ErrTimeoutElapsed(height clienttypes.Height, timestamp uint64) error {
	if t.heightElapsed(height) {
		return errorsmod.Wrapf(ErrTimeoutElapsed, "current height: %s, timeout height %s", height, t.Height)
	}

	return errorsmod.Wrapf(ErrTimeoutElapsed, "current timestamp: %d, timeout timestamp %d", timestamp, t.Timestamp)
}

// ErrTimeoutNotReached returns a timeout not reached error indicating which timeout value
// has not been reached.
func (t Timeout) ErrTimeoutNotReached(height clienttypes.Height, timestamp uint64) error {
	// only return height information if the height is set
	// t.heightElapsed() will return false when it is empty
	if !t.Height.IsZero() && !t.heightElapsed(height) {
		return errorsmod.Wrapf(ErrTimeoutNotReached, "current height: %s, timeout height %s", height, t.Height)
	}

	return errorsmod.Wrapf(ErrTimeoutNotReached, "current timestamp: %d, timeout timestamp %d", timestamp, t.Timestamp)
}

// heightElapsed returns true if the timeout height is non empty
// and the timeout height is greater than or equal to the relative height.
func (t Timeout) heightElapsed(height clienttypes.Height) bool {
	return !t.Height.IsZero() && height.GTE(t.Height)
}

// timestampElapsed returns true if the timeout timestamp is non empty
// and the timeout timestamp is greater than or equal to the relative timestamp.
func (t Timeout) timestampElapsed(timestamp uint64) bool {
	return t.Timestamp != 0 && timestamp >= t.Timestamp
}
