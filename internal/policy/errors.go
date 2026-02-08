package policy

import "errors"

var (
	// ErrPolicyDenied indicates the tool call was denied by policy.
	ErrPolicyDenied = errors.New("policy: tool call denied")

	// ErrApprovalTimeout indicates the approval request timed out.
	ErrApprovalTimeout = errors.New("approval: request timed out")

	// ErrApprovalRejected indicates the request was rejected by user.
	ErrApprovalRejected = errors.New("approval: request rejected")

	// ErrInvalidPattern indicates a malformed regex pattern.
	ErrInvalidPattern = errors.New("policy: invalid regex pattern")

	// ErrRequestNotFound indicates the approval request was not found.
	ErrRequestNotFound = errors.New("approval: request not found")

	// ErrMaxPendingExceeded indicates too many pending approval requests.
	ErrMaxPendingExceeded = errors.New("approval: max pending requests exceeded")
)
