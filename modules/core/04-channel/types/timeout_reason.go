// SPDX-License-Identifier: Apache-2.0

package types

// TimeoutReason represents the reason why a packet timed out.
// It is returned by the packet timeout path so that callers can query
// which timeout condition elapsed or whether the proof verification failed.
type TimeoutReason string

const (
	// HEIGHT_TIMEOUT indicates the packet timed out because the timeout height elapsed.
	HEIGHT_TIMEOUT TimeoutReason = "HEIGHT_TIMEOUT"
	// TIMESTAMP_TIMEOUT indicates the packet timed out because the timeout timestamp elapsed.
	TIMESTAMP_TIMEOUT TimeoutReason = "TIMESTAMP_TIMEOUT"
	// INVALID_PROOF indicates the packet timeout proof verification failed.
	INVALID_PROOF TimeoutReason = "INVALID_PROOF"
)

// String returns the string representation of the timeout reason.
func (r TimeoutReason) String() string {
	return string(r)
}
