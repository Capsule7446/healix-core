# v0.6 Fault Code Registry

This registry defines the stable public error-code contract. A code is immutable once published: its owner, `Kind`, meaning, required safe detail schema, and retry meaning cannot change or be reused. `Message` is a safe English fallback, not an i18n key or a stable text protocol.

## Registry rules

- Code format: `UPPER_SNAKE_CASE`, with a bounded-context prefix.
- `EXECUTION_*` owns node runtime, engine, scheduling, and execution-application failures.
- Allowed parameters are fixed per code, lower-camel-case, locale-neutral, bounded, and never secrets, selectors, environment/parameter values, URLs, command payloads, stack traces, or causes.
- Violations are allowed only on aggregate input codes and are ordered deterministically.
- Unregistered production fault codes, duplicate codes, cross-context prefixes, and public `errors.New` sentinels are contract violations.

## Execution

| Code | Kind | Safe message | Allowed params | Retry / notes |
|---|---|---|---|---|
| `EXECUTION_ELEMENT_NOT_FOUND` | `NOT_FOUND` | `element was not found` | none | Existing healing policy decides recovery. |
| `EXECUTION_OPERATION_CANCELED` | `CANCELED` | `node operation was canceled` | none | Must preserve `context.Canceled` in chain. |
| `EXECUTION_OPERATION_TIMEOUT` | `DEADLINE_EXCEEDED` | `node operation timed out` | none | Must preserve deadline cause. |
| `EXECUTION_TRANSIENT_DRIVER` | `UNAVAILABLE` | `node driver is temporarily unavailable` | none | Explicit retryable driver classification only. |
| `EXECUTION_OPERATION_FAILED` | `INTERNAL` | `node operation failed` | none | Cause is never public. |
| `EXECUTION_STEP_TIMELINE_START_FAILED` | `INTERNAL` | `step timeline start could not be recorded` | none | Preserve the recorder cause privately; node identity, occurrence, and adapter details remain private. |
| `EXECUTION_STEP_TIMELINE_FINISH_FAILED` | `INTERNAL` | `step timeline finish could not be recorded` | none | Preserve validation or recorder causes privately; node identity, occurrence, and timeline values remain private. |
| `EXECUTION_NODE_COMPLETION_OBSERVATION_FAILED` | `INTERNAL` | `node completion observation could not be recorded` | none | Preserve the observer cause privately; execution identities, handler results, and adapter details remain private. |
| `EXECUTION_LEAF_COMPLETION_FAILED` | `INTERNAL` | `leaf execution completion failed` | none | Aggregate node and completion side-effect failures without exposing any underlying message; every cause remains traversable privately. |
| `EXECUTION_PLAN_UNSEALED` | `FAILED_PRECONDITION` | `execution plan must be sealed` | none | Not retryable without sealing. |
| `EXECUTION_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `execution status transition is invalid` | none | The lifecycle state is not itself a fault. |
| `EXECUTION_RUN_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `run status transition is invalid` | none | The containing run lifecycle is not itself a fault. |
| `EXECUTION_WORKER_FENCE_STALE` | `CONFLICT` | `worker execution authority is stale` | none | Re-read/claim authority; no raw fence value. |
| `EXECUTION_ENTRY_STATES_INVALID` | `FAILED_PRECONDITION` | `execution entry states are invalid` | none | Scheduling state must match the sealed run plan and serial lifecycle constraints. |
| `EXECUTION_FACT_COMMITTER_REQUIRED` | `FAILED_PRECONDITION` | `execution fact committer is required` | none | The caller must provide both transaction and governance dependencies. |
| `EXECUTION_AUTHORITY_VERIFIER_REQUIRED` | `FAILED_PRECONDITION` | `execution authority verifier is required` | none | Execution requires an authority verifier before it can run side effects. |
| `EXECUTION_IDENTITY_MISMATCH` | `FAILED_PRECONDITION` | `execution identity does not match the sealed entry` | none | Rebuild execution configuration from the authoritative sealed entry; no identity/token values are exposed. |
| `EXECUTION_TIMELINE_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution timeline configuration is invalid` | none | Provide a recorder and a non-nil recorder timeline when step timeline recording is enabled. |
| `EXECUTION_COMPLETION_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution completion configuration is invalid` | none | Provide a read-only browser before enabling completion handlers. |
| `EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED` | `FAILED_PRECONDITION` | `execution scheduling dependency is required` | none | Supply all scheduling ports before processing claims. |
| `EXECUTION_SCHEDULING_CLAIM_INVALID` | `FAILED_PRECONDITION` | `scheduling claim is invalid` | none | Release the claim and obtain a valid authoritative claim; run and claim identifiers remain private. |
| `EXECUTION_CREATE_RUN_COMMAND_INVALID` | `INVALID_ARGUMENT` | `create-run command is invalid` | none | Correct the command before retrying; command identifiers, payload values, and validation details remain private. |
| `EXECUTION_CREATE_RUN_COMMAND_CONFLICT` | `CONFLICT` | `create-run command conflicts with an existing request` | none | Reconcile the existing command or use a new command ID for a semantically new request; command IDs, request digests, and payload values remain private. |
| `EXECUTION_CREATE_RUN_SNAPSHOT_CONFLICT` | `CONFLICT` | `create-run snapshot conflicts with the authoritative run` | none | Re-read the authoritative run before retrying; run identities, snapshots, and digest values remain private. |
| `EXECUTION_CREATE_RUN_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `create-run adapter returned an invalid authoritative result` | none | The adapter result is malformed or inconsistent; retain validation causes privately and expose no adapter data, identifiers, digests, snapshots, catalog details, or payload values. |
| `EXECUTION_CREATE_RUN_RETRYABLE` | `UNAVAILABLE` | `create-run outcome is temporarily unavailable` | none | Retry the unchanged request using the same command ID because a transaction outcome can be unknown; command IDs, transaction state, and raw causes remain private. |
| `EXECUTION_CREATE_RUN_CATALOG_GRAPH_UNRESOLVABLE` | `FAILED_PRECONDITION` | `create-run catalog graph is unavailable or invalid` | none | Refresh or correct the referenced catalog graph before retrying; graph operations, identities, paths, bindings, values, and raw causes remain private. |
| `EXECUTION_CANCEL_RUN_COMMAND_INVALID` | `INVALID_ARGUMENT` | `cancel run command is invalid` | none | Correct the command before retrying; command fields and values remain private. |
| `EXECUTION_ABORT_RUN_COMMAND_INVALID` | `INVALID_ARGUMENT` | `abort run command is invalid` | none | Correct the command and worker fence before retrying; command fields, values, and claim tokens remain private. |
| `EXECUTION_REORDER_QUEUE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `reorder queue command is invalid` | none | Correct the queue reorder command before retrying; command fields, scope, and run identifiers remain private. |
| `EXECUTION_RUN_SIGNAL_RETRYABLE` | `UNAVAILABLE` | `execution cancellation signal must be retried` | none | The terminal state is committed; retry only the external cancellation signal and retain its cause privately. |
| `EXECUTION_STEP_REVISION_CONFLICT` | `CONFLICT` | `step transition revision conflicts with current state` | none | Re-read authoritative execution evidence state before retrying; revision values remain private. |
| `EXECUTION_STEP_TRANSITION_COMMIT_IDENTITY_CONFLICT` | `CONFLICT` | `step transition commit identity conflicts with the previously accepted commit` | none | A changed replay is rejected; commit identity and payload details remain private. |
| `EXECUTION_RUN_COMMAND_IDENTITY_CONFLICT` | `CONFLICT` | `run command identity conflicts with an existing request` | none | A changed command replay is rejected without exposing command identity or payload details. |
| `EXECUTION_RUN_IDENTITY_CONFLICT` | `CONFLICT` | `run identity conflicts with the authoritative state` | none | Re-read the authoritative run before retrying; run identity remains private. |
| `EXECUTION_RUN_REVISION_CONFLICT` | `CONFLICT` | `run revision conflicts with current state` | none | Re-read the authoritative run before retrying; revision values remain private. |
| `EXECUTION_RUN_STATUS_CONFLICT` | `CONFLICT` | `run status conflicts with current state` | none | Re-read the authoritative run lifecycle before retrying; status values remain private. |
| `EXECUTION_QUEUE_REVISION_CONFLICT` | `CONFLICT` | `queue revision conflicts with current state` | none | Re-read the authoritative queue before retrying; scope and revision values remain private. |
| `EXECUTION_QUEUE_MEMBERSHIP_CONFLICT` | `CONFLICT` | `queue membership conflicts with the authoritative state` | none | Re-read the authoritative queue membership before retrying; scope and run identities remain private. |
| `EXECUTION_RUN_COMMAND_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `run command adapter returned an invalid authoritative result` | none | Preserve the validation cause only for diagnostics; public output never includes adapter details, identities, revisions, statuses, or payloads. |
| `EXECUTION_HEAL_GOVERNANCE_SNAPSHOT_INVALID` | `FAILED_PRECONDITION` | `heal governance snapshot is invalid` | none | Repair or reload malformed persisted governance state before retrying; snapshot identities, revisions, candidate states, streak details, and private causes remain private. |
| `EXECUTION_HEAL_ACCEPTED_FACT_INVALID` | `INVALID_ARGUMENT` | `accepted heal fact is invalid` | none | Correct the accepted fact before retrying; fact kinds, provenance, payloads, decision bands, and identities remain private. |
| `EXECUTION_HEAL_TERMINAL_EFFECT_CONFLICT` | `CONFLICT` | `heal terminal effect conflicts with persisted state` | none | Re-read and reconcile the persisted terminal effect; effect kinds, candidate hashes, bands, contributions, and private causes remain private. |

## Automation

| Code | Kind | Safe message | Allowed params / violations | Notes |
|---|---|---|---|---|
| `AUTOMATION_TEST_TASK_INVALID` | `INVALID_ARGUMENT` | `test task input is invalid` | ordered typed violations only | Aggregate validation envelope. |
| `AUTOMATION_FOLDER_NOT_FOUND` | `NOT_FOUND` | `automation folder was not found` | none | Do not expose authorization-sensitive detail. |
| `AUTOMATION_AGGREGATE_DELETED` | `FAILED_PRECONDITION` | `automation aggregate has been deleted` | none | Mutations require a restored aggregate; no aggregate identity is exposed. |
| `AUTOMATION_VERSION_NUMBER_EXHAUSTED` | `RESOURCE_EXHAUSTED` | `automation version number capacity is exhausted` | none | Publishing cannot continue until version capacity is remediated. |
| `AUTOMATION_PERSISTED_VERSION_NUMBER_INVALID` | `FAILED_PRECONDITION` | `persisted version number must be positive` | none | Reject corrupt persisted version state before deriving the next version; version identities and values remain private. |
| `AUTOMATION_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `automation service is not configured` | none | The caller must provide valid service dependencies; no dependency identity is exposed. |
| `AUTOMATION_HEAL_CANDIDATE_STALE_BASE` | `CONFLICT` | `heal candidate base version is no longer current` | none | Refresh the candidate and current element target before retrying; no candidate/version identifiers are exposed. |
| `AUTOMATION_REVISION_CONFLICT` | `CONFLICT` | `automation revision conflicts with current state` | none | Re-read authoritative automation state before retrying; aggregate identities and revision values remain private. |
| `AUTOMATION_PERSISTED_REVISION_INVALID` | `FAILED_PRECONDITION` | `persisted revision must be non-zero` | none | Reject corrupt or unpersisted revision state before mutation; aggregate identities and revision values remain private. |
| `AUTOMATION_REVISION_EXHAUSTED` | `RESOURCE_EXHAUSTED` | `revision value is exhausted` | none | The finite revision namespace has no successor; do not retry or wrap to zero, and perform no persistence write. |
| `AUTOMATION_HEAL_CANDIDATE_IDENTITY_INVALID` | `INVALID_ARGUMENT` | `heal candidate identity is invalid` | none | Correct the candidate identity before retrying; candidate, node, and version identities remain private. |
| `AUTOMATION_HEAL_CANDIDATE_STATE_INVALID` | `FAILED_PRECONDITION` | `heal candidate state does not allow this operation` | none | Refresh the candidate before validating or applying a lifecycle operation; candidate identity and status remain private. |
| `AUTOMATION_HEAL_CANDIDATE_REVIEW_STATUS_INVALID` | `INVALID_ARGUMENT` | `heal candidate review status is invalid` | none | Supply a supported review decision; the supplied status and candidate payload remain private. |
| `AUTOMATION_HEAL_CANDIDATE_REVIEW_COMMAND_INVALID` | `INVALID_ARGUMENT` | `heal candidate review command is invalid` | none | Correct the command identity before retrying; command, candidate, node, and version identities remain private. |
| `AUTOMATION_HEAL_APPROVAL_STATUS_INVALID` | `INVALID_ARGUMENT` | `heal approval status is invalid` | none | Supply an approved or rejected decision; the supplied value and command payload remain private. |
| `AUTOMATION_HEAL_DECISION_BAND_INVALID` | `INVALID_ARGUMENT` | `heal decision band is invalid` | none | Supply a governance band consistent with candidate presence; candidate identity and the supplied band remain private. |
| `AUTOMATION_HEAL_CONFIDENCE_INVALID` | `INVALID_ARGUMENT` | `heal confidence is invalid` | none | Supply a finite confidence in the inclusive range from zero through one; the supplied numeric value remains private. |
| `AUTOMATION_HEAL_STREAK_STATE_INVALID` | `FAILED_PRECONDITION` | `persisted heal streak state is invalid` | none | Repair or discard corrupt persisted streak state before processing another observation; identities, dispositions, sequences, and contribution details remain private. |
| `AUTOMATION_HEAL_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `heal observation is invalid` | none | Correct the incoming observation before retrying; provenance, candidate governance, outcome, and identity values remain private. |
| `AUTOMATION_HEAL_SEQUENCE_CONFLICT` | `CONFLICT` | `heal sequence conflicts with persisted ordering` | none | Re-read persisted streak ordering before retrying; incoming and persisted sequence values remain private and state is unchanged. |
| `AUTOMATION_HEAL_PROVENANCE_CONFLICT` | `CONFLICT` | `heal observation conflicts with persisted provenance` | none | Reconcile duplicate fact, commit, run, or sequence provenance before retrying; all provenance values remain private and state is unchanged. |
| `AUTOMATION_HEAL_STREAK_REJECTION_INVALID` | `FAILED_PRECONDITION` | `heal streak cannot be rejected in its current state` | none | Refresh the streak and reject only an await-approval state; disposition and candidate identity remain private. |
| `AUTOMATION_HEAL_REVIEW_IDENTITY_CONFLICT` | `CONFLICT` | `heal review command conflicts with an existing request` | none | Reconcile the existing command or use a new command ID; command IDs, request digests, and review payloads remain private. |
| `AUTOMATION_HEAL_REVIEW_DECISION_CONFLICT` | `FAILED_PRECONDITION` | `heal candidate is no longer available for review` | none | Refresh the candidate before attempting another decision; candidate identity and state remain private. |
| `AUTOMATION_HEAL_REVIEW_AUTHORITY_CONFLICT` | `CONFLICT` | `heal review authority changed before the operation completed` | none | Re-read candidate, node, and streak authority before reconciling; identities, revisions, and streak state remain private. |
| `AUTOMATION_HEAL_REVIEW_CONTRACT_VIOLATION` | `INTERNAL` | `heal review could not be completed` | none | The adapter outcome is malformed; retain causes privately and expose no review identities, payloads, authority state, or adapter details. |
| `SAMPLING_PUBLICATION_IDENTITY_CONFLICT` | `CONFLICT` | `sampling publication identity conflicts with an existing request` | none | A replay with the same publication identity but a different request digest is rejected without exposing identity or digest values. |
| `AUTOMATION_SAMPLING_PUBLICATION_DIGEST_MISMATCH` | `INVALID_ARGUMENT` | `sampling publication digest does not match the request payload` | none | Reject before any sampling-publication transaction operation; request digests, publication identities, and payload values remain private. |
| `AUTOMATION_SAMPLING_PUBLICATION_UNAVAILABLE` | `UNAVAILABLE` | `sampling publication service is unavailable` | none | Supply a valid transaction dependency before retrying; dependency details remain private. |
| `AUTOMATION_SAMPLING_PUBLICATION_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `sampling publication adapter returned an invalid outcome` | none | The adapter outcome is malformed; preserve its cause only for diagnostics and do not expose outcome, identity, digest, or payload details. |
| `AUTOMATION_SAMPLING_PUBLICATION_AUTHORITY_CONFLICT` | `CONFLICT` | `sampling publication authority changed before the publication could be applied` | none | Re-read node authority before retrying; conflict preserves rollback and does not disclose publication or revision details. |

## Sampling, evidence, fingerprint, interpolation

| Code | Kind | Safe message | Allowed params / violations | Notes |
|---|---|---|---|---|
| `FINGERPRINT_SELECTOR_INVALID` | `INVALID_ARGUMENT` | `element selector is invalid` | none | Selector source values are never exposed in the public message. |
| `INTERPOLATION_RESOLVER_REQUIRED` | `FAILED_PRECONDITION` | `variable resolver is required` | none | A resolver is needed only when interpolation syntax is present. |
| `INTERPOLATION_EXPRESSION_INVALID` | `INVALID_ARGUMENT` | `variable expression is invalid` | none | Source expression and variable name never enter the public message. |
| `INTERPOLATION_VARIABLE_UNDEFINED` | `NOT_FOUND` | `referenced variable is not defined` | none | Variable name and source expression remain private. |

These families are added only in the atomic bounded-context migration that introduces the corresponding producer. Every addition must include its `Kind`, fixed safe fallback message, allowed parameter/violation schema, retry behavior, public consumer, and any persistence mapping.

## Historical execution evidence mapping

Host persistence migration is required for values formerly recorded as node `ErrorKind`:

| v0.5 persisted kind | v0.6 kind | v0.6 code |
|---|---|---|
| `not_found` | `NOT_FOUND` | `EXECUTION_ELEMENT_NOT_FOUND` |
| `timeout` | `DEADLINE_EXCEEDED` | `EXECUTION_OPERATION_TIMEOUT` |
| `canceled` | `CANCELED` | `EXECUTION_OPERATION_CANCELED` |
| `transient` | `UNAVAILABLE` | `EXECUTION_TRANSIENT_DRIVER` |
| `unknown` / unclassified | `INTERNAL` | `EXECUTION_OPERATION_FAILED` |

The Core writes only v0.6 `Kind + Code` facts after migration. SQLite/schema conversion or dual-read behavior belongs to the Host and must be independently verified there.
