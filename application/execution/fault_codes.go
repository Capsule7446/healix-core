package execution

import "github.com/Capsule7446/healix-core/domain/fault"

// Every fault.Code this package publishes is declared here, which is what
// AGENTS.md asks of each package and what all seven domain packages already do.
//
// The single entry point is the point. Codes are a public compatibility
// contract: they may only be added or tombstoned, never renamed or reused, and
// an auditor checking that has to be able to see the whole set at once. While
// these lived beside the features that raise them, "all of them" meant grepping
// seven files and trusting the grep.
//
// Blocks stay grouped by the feature that raises them, and each keeps the
// rationale for its Kind, because the remediation a code promises is what makes
// it choosable -- see docs/contracts/error-code-registry.md, which
// architecture/fault_contract_guard_test.go parses line by line against these
// declarations.

// --- ports.go ---

const (
	CodeFactCommitterRequired  fault.Code = "EXECUTION_FACT_COMMITTER_REQUIRED"
	CodeStepRevisionConflict   fault.Code = "EXECUTION_STEP_REVISION_CONFLICT"
	CodeCommitIdentityConflict fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_IDENTITY_CONFLICT"
	// CodeStepTransitionCommitPayloadTooLarge covers both the overall payload
	// budget and any one string exceeding its own byte limit: the remediation is
	// always to shrink the commit, never to correct a specific field's value.
	CodeStepTransitionCommitPayloadTooLarge fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_PAYLOAD_TOO_LARGE"
	// CodeStepTransitionCommitRunMismatch covers a commit fact whose own InstanceID
	// disagrees with the claimed worker fence's InstanceID. FAILED_PRECONDITION: the
	// caller must re-read the authoritative claim, not supply a different value.
	CodeStepTransitionCommitRunMismatch fault.Code = "EXECUTION_STEP_TRANSITION_COMMIT_RUN_MISMATCH"
)

// --- entry_completion_transaction.go ---

const (
	// CodeCompleteEntryCommandInvalid covers a completion request core cannot
	// digest: a malformed identity, fence, observed state or engine outcome. The
	// caller must repair the request, hence INVALID_ARGUMENT.
	CodeCompleteEntryCommandInvalid fault.Code = "EXECUTION_COMPLETE_ENTRY_COMMAND_INVALID"
	// CodeCompleteEntryDigestMismatch covers an intent whose digest or decision
	// does not follow from its own command. It is what stops a host from
	// substituting a decision core never made.
	CodeCompleteEntryDigestMismatch fault.Code = "EXECUTION_COMPLETE_ENTRY_DIGEST_MISMATCH"
	// CodeCompleteEntryUnavailable covers a service with no transaction behind
	// it. Nothing about the request is wrong, so it is UNAVAILABLE rather than
	// INVALID_ARGUMENT.
	CodeCompleteEntryUnavailable fault.Code = "EXECUTION_COMPLETE_ENTRY_UNAVAILABLE"
	// CodeCompleteEntryAdapterContractViolation covers an adapter that returned
	// an outcome the port forbids — an unknown status, a different identity, or
	// a decision it recomputed for itself. That is an implementation defect in
	// the adapter, not a business failure, hence INTERNAL.
	CodeCompleteEntryAdapterContractViolation fault.Code = "EXECUTION_COMPLETE_ENTRY_ADAPTER_CONTRACT_VIOLATION"
	// CodeCompleteEntryIdentityConflict is the code adapters raise when the
	// entry no longer holds the state the command claimed to observe, so the
	// compare-and-swap found a different writer had moved first.
	CodeCompleteEntryIdentityConflict fault.Code = "EXECUTION_COMPLETE_ENTRY_IDENTITY_CONFLICT"
)

// --- abort_request_transaction.go ---

const (
	// CodeRequestAbortCommandInvalid covers an abort request core cannot digest:
	// a malformed identity, fence, observed state or request. The caller must
	// repair the request, hence INVALID_ARGUMENT.
	CodeRequestAbortCommandInvalid fault.Code = "EXECUTION_REQUEST_ABORT_COMMAND_INVALID"
	// CodeRequestAbortDigestMismatch covers an intent whose digest or decision
	// does not follow from its own command. It is what stops a host from
	// substituting counters core never produced.
	CodeRequestAbortDigestMismatch fault.Code = "EXECUTION_REQUEST_ABORT_DIGEST_MISMATCH"
	// CodeRequestAbortUnavailable covers a service with no transaction behind
	// it. Nothing about the request is wrong, so it is UNAVAILABLE rather than
	// INVALID_ARGUMENT.
	CodeRequestAbortUnavailable fault.Code = "EXECUTION_REQUEST_ABORT_UNAVAILABLE"
	// CodeRequestAbortAdapterContractViolation covers an adapter that returned
	// an outcome the port forbids — an unknown status, a different identity, or
	// a decision it recomputed for itself. That is an implementation defect in
	// the adapter, not a business failure, hence INTERNAL.
	CodeRequestAbortAdapterContractViolation fault.Code = "EXECUTION_REQUEST_ABORT_ADAPTER_CONTRACT_VIOLATION"
	// CodeRequestAbortIdentityConflict is the code adapters raise when the entry
	// no longer holds the state the command claimed to observe, so the
	// compare-and-swap found a different writer had moved first.
	CodeRequestAbortIdentityConflict fault.Code = "EXECUTION_REQUEST_ABORT_IDENTITY_CONFLICT"
)

// --- entry_completion.go ---

const (
	// CodeEntryCompletionStateInvalid covers a malformed observed state: an
	// unknown entry status or terminal intent, or a negative revision. The caller
	// read something the core vocabulary does not contain, so the remediation is
	// to repair the read, hence INVALID_ARGUMENT.
	CodeEntryCompletionStateInvalid fault.Code = "EXECUTION_ENTRY_COMPLETION_STATE_INVALID"
	// CodeEntryCompletionRevisionExhausted covers a state whose successor
	// revision core would refuse to write. Nothing about the argument is
	// malformed; the counter simply has no room left, hence OUT_OF_RANGE.
	CodeEntryCompletionRevisionExhausted fault.Code = "EXECUTION_ENTRY_COMPLETION_REVISION_EXHAUSTED"
	// CodeEntryCompletionNotRunning covers a well-formed state that is not
	// RUNNING. Only a running entry can be completed, and the caller must
	// re-read the entry before retrying, hence FAILED_PRECONDITION.
	CodeEntryCompletionNotRunning fault.Code = "EXECUTION_ENTRY_COMPLETION_NOT_RUNNING"
	// CodeEngineOutcomeInvalid covers an engine outcome outside the engine
	// vocabulary, or a failure code that is blank without being absent.
	CodeEngineOutcomeInvalid fault.Code = "EXECUTION_ENGINE_OUTCOME_INVALID"
)

// --- heal_governance.go ---

const (
	CodeHealGovernanceSnapshotInvalid fault.Code = "EXECUTION_HEAL_GOVERNANCE_SNAPSHOT_INVALID"
	CodeHealAcceptedFactInvalid       fault.Code = "EXECUTION_HEAL_ACCEPTED_FACT_INVALID"
	CodeHealTerminalEffectConflict    fault.Code = "EXECUTION_HEAL_TERMINAL_EFFECT_CONFLICT"
)

// --- entry_executor.go ---

const (
	// CodeEntryExecutorConfigurationInvalid covers NewEntryExecutor's own
	// constructor checks: none of these are a caller argument distinct from the
	// executor's own configuration, so the remediation is always to repair that
	// configuration before construction, hence FAILED_PRECONDITION.
	CodeEntryExecutorConfigurationInvalid fault.Code = "EXECUTION_ENTRY_EXECUTOR_CONFIGURATION_INVALID"
	// CodeSchedulingAdapterUnavailable covers a browser session factory failure:
	// the host adapter, not the caller, needs to become reachable again.
	CodeSchedulingAdapterUnavailable fault.Code = "EXECUTION_SCHEDULING_ADAPTER_UNAVAILABLE"
	// CodeEntryBrowserSessionAdapterContractViolation covers a nil or invalid
	// session returned by the host factory: the factory itself violated its
	// contract, which has no caller remediation.
	CodeEntryBrowserSessionAdapterContractViolation fault.Code = "EXECUTION_ENTRY_BROWSER_SESSION_ADAPTER_CONTRACT_VIOLATION"
)

// --- abort_request.go ---

const (
	// CodeAbortRequestInvalid covers a malformed request: a pending command id
	// that is absent, blank, or carries surrounding space. The identity reaches
	// the host's idempotency row verbatim, so a value that cannot be traced back
	// to a command is repaired by the caller, hence INVALID_ARGUMENT.
	CodeAbortRequestInvalid fault.Code = "EXECUTION_ABORT_REQUEST_INVALID"
	// CodeAbortRequestNotRunning covers a well-formed state that is not RUNNING.
	// Only a running entry can be asked to stop; anything else has already
	// reached a terminal status or has not started, and the caller must re-read
	// before retrying, hence FAILED_PRECONDITION.
	//
	// It is a separate code from CodeEntryCompletionNotRunning because the
	// remediation differs: a completion that arrives late is usually a replay to
	// be looked up, while an abort request against a finished entry is a request
	// with nothing left to act on.
	CodeAbortRequestNotRunning fault.Code = "EXECUTION_ABORT_REQUEST_NOT_RUNNING"
	// CodeAbortRequestAlreadyAborting covers a request against an entry whose
	// intent is already ABORT. The abort is in flight and there is nothing left
	// to advance, hence FAILED_PRECONDITION rather than a silent success.
	CodeAbortRequestAlreadyAborting fault.Code = "EXECUTION_ABORT_REQUEST_ALREADY_ABORTING"
)
