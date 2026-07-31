# v0.6 Fault Code Registry

This registry defines the stable public error-code contract. A code is immutable once published: its owner, `Kind`, meaning, required safe detail schema, and retry meaning cannot change or be reused. `Message` is a safe English fallback, not an i18n key or a stable text protocol.

## Registry rules

- Code format: `UPPER_SNAKE_CASE`, with a bounded-context prefix.
- `EXECUTION_*` owns node runtime, engine, scheduling, and execution-application failures.
- Allowed parameters are fixed per code, lower-camel-case, locale-neutral, bounded, and never secrets, selectors, environment/parameter values, URLs, command payloads, stack traces, or causes.
- Violations are allowed only on aggregate input codes and are ordered deterministically.
- Violation reason codes are owned by the shared kernel and listed under "Violation codes". They are the only codes without a bounded-context prefix, and they may appear only as a `Violation` code, never as a top-level `Error` code.
- Unregistered production fault codes, duplicate codes, cross-context prefixes, and public `errors.New` sentinels are contract violations.

## Violation codes

Owned by `domain/fault` and shared by every context's aggregate validation envelope. A violation's `field` says *which* input failed; its code says *why*. The vocabulary stays closed and small on purpose: minting a code per failing field would multiply frontend i18n keys without adding meaning, which is exactly what the aggregate envelope exists to prevent.

| Code | Kind of failure | Notes |
|---|---|---|
| `VALIDATION_FIELD_REQUIRED` | A mandatory input is absent or blank. | Remediation is to supply the field named by `field`. |
| `VALIDATION_FIELD_INVALID` | A present input holds an unacceptable value. | Covers range, format, enum, and ordering rules. The rejected value stays private. |
| `VALIDATION_FIELD_DUPLICATE` | An input repeats a value required to be unique. | `field` points at the later occurrence, not the first. The repeated value stays private. |
| `VALIDATION_FIELD_MISMATCH` | An input contradicts the aggregate holding it. | Covers wrong owner, wrong parent, and policy/value contradictions. |

- `field` is a logical, locale-neutral path matching `^[a-z][A-Za-z0-9.]{0,126}$`. It names the public contract vocabulary, not internal struct fields.
- Collection indexes in `field` are **0-based** and address the collection the caller passed.
- Violation `message` and `params` obey the same safety rules as top-level codes: no identities, keys, enum values, causes, or user input.
- One envelope carries at most `fault.MaxViolations` violations. Past the cap the deterministic leading prefix is kept and the remainder dropped, so a consumer must not read the violation count as complete.

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
| `EXECUTION_INSTANCE_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `instance status transition is invalid` | none | The containing instance lifecycle is not itself a fault. |
| `EXECUTION_WORKER_FENCE_STALE` | `CONFLICT` | `worker execution authority is stale` | none | Re-read/claim authority; no raw fence value. |
| `EXECUTION_ENTRY_STATES_INVALID` | `FAILED_PRECONDITION` | `execution entry states are invalid` | none | Scheduling state must match the sealed run plan and serial lifecycle constraints. |
| `EXECUTION_FACT_COMMITTER_REQUIRED` | `FAILED_PRECONDITION` | `execution fact committer is required` | none | The caller must provide both transaction and governance dependencies. |
| `EXECUTION_AUTHORITY_VERIFIER_REQUIRED` | `FAILED_PRECONDITION` | `execution authority verifier is required` | none | Execution requires an authority verifier before it can run side effects. |
| `EXECUTION_IDENTITY_MISMATCH` | `FAILED_PRECONDITION` | `execution identity does not match the sealed entry` | none | Rebuild execution configuration from the authoritative sealed entry; no identity/token values are exposed. |
| `EXECUTION_TIMELINE_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution timeline configuration is invalid` | none | Provide a recorder and a non-nil recorder timeline when step timeline recording is enabled. |
| `EXECUTION_COMPLETION_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `execution completion configuration is invalid` | none | Provide a read-only browser before enabling completion handlers. |
| `EXECUTION_SCHEDULING_DEPENDENCY_REQUIRED` | `FAILED_PRECONDITION` | `execution scheduling dependency is required` | none | Supply all scheduling ports before processing claims. |
| `EXECUTION_SCHEDULING_CLAIM_INVALID` | `FAILED_PRECONDITION` | `scheduling claim is invalid` | none | Release the claim and obtain a valid authoritative claim; run and claim identifiers remain private. |
| `EXECUTION_CREATE_INSTANCE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `create-instance command is invalid` | none | Correct the command before retrying; command identifiers, payload values, and validation details remain private. |
| `EXECUTION_CREATE_INSTANCE_COMMAND_CONFLICT` | `CONFLICT` | `create-instance command conflicts with an existing request` | none | Reconcile the existing command or use a new command ID for a semantically new request; command IDs, request digests, and payload values remain private. |
| `EXECUTION_CREATE_INSTANCE_SNAPSHOT_CONFLICT` | `CONFLICT` | `create-instance snapshot conflicts with the authoritative instance` | none | Re-read the authoritative instance before retrying; instance identities, snapshots, and digest values remain private. |
| `EXECUTION_CREATE_INSTANCE_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `create-instance adapter returned an invalid authoritative result` | none | The adapter result is malformed or inconsistent; retain validation causes privately and expose no adapter data, identifiers, digests, snapshots, catalog details, or payload values. |
| `EXECUTION_CREATE_INSTANCE_RETRYABLE` | `UNAVAILABLE` | `create-instance outcome is temporarily unavailable` | none | Retry the unchanged request using the same command ID because a transaction outcome can be unknown; command IDs, transaction state, and raw causes remain private. |
| `EXECUTION_CREATE_INSTANCE_CATALOG_GRAPH_UNRESOLVABLE` | `FAILED_PRECONDITION` | `create-instance catalog graph is unavailable or invalid` | none | Refresh or correct the referenced catalog graph before retrying; graph operations, identities, paths, bindings, values, and raw causes remain private. |
| `EXECUTION_CANCEL_INSTANCE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `cancel instance command is invalid` | none | Correct the command before retrying; command fields and values remain private. |
| `EXECUTION_ABORT_INSTANCE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `abort instance command is invalid` | none | Correct the command and worker fence before retrying; command fields, values, and claim tokens remain private. |
| `EXECUTION_REORDER_QUEUE_COMMAND_INVALID` | `INVALID_ARGUMENT` | `reorder queue command is invalid` | none | Correct the queue reorder command before retrying; command fields, scope, and run identifiers remain private. |
| `EXECUTION_INSTANCE_SIGNAL_RETRYABLE` | `UNAVAILABLE` | `execution cancellation signal must be retried` | none | The terminal state is committed; retry only the external cancellation signal and retain its cause privately. |
| `EXECUTION_STEP_REVISION_CONFLICT` | `CONFLICT` | `step transition revision conflicts with current state` | none | Re-read authoritative execution evidence state before retrying; revision values remain private. |
| `EXECUTION_STEP_TRANSITION_COMMIT_IDENTITY_CONFLICT` | `CONFLICT` | `step transition commit identity conflicts with the previously accepted commit` | none | A changed replay is rejected; commit identity and payload details remain private. |
| `EXECUTION_INSTANCE_COMMAND_IDENTITY_CONFLICT` | `CONFLICT` | `instance command identity conflicts with an existing request` | none | A changed command replay is rejected without exposing command identity or payload details. |
| `EXECUTION_INSTANCE_IDENTITY_CONFLICT` | `CONFLICT` | `instance identity conflicts with the authoritative state` | none | Re-read the authoritative instance before retrying; instance identity remains private. |
| `EXECUTION_INSTANCE_REVISION_CONFLICT` | `CONFLICT` | `instance revision conflicts with current state` | none | Re-read the authoritative instance before retrying; revision values remain private. |
| `EXECUTION_INSTANCE_STATUS_CONFLICT` | `CONFLICT` | `instance status conflicts with current state` | none | Re-read the authoritative instance lifecycle before retrying; status values remain private. |
| `EXECUTION_QUEUE_REVISION_CONFLICT` | `CONFLICT` | `queue revision conflicts with current state` | none | Re-read the authoritative queue before retrying; scope and revision values remain private. |
| `EXECUTION_QUEUE_MEMBERSHIP_CONFLICT` | `CONFLICT` | `queue membership conflicts with the authoritative state` | none | Re-read the authoritative queue membership before retrying; scope and run identities remain private. |
| `EXECUTION_INSTANCE_COMMAND_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `instance command adapter returned an invalid authoritative result` | none | Preserve the validation cause only for diagnostics; public output never includes adapter details, identities, revisions, statuses, or payloads. |
| `EXECUTION_HEAL_GOVERNANCE_SNAPSHOT_INVALID` | `FAILED_PRECONDITION` | `heal governance snapshot is invalid` | none | Repair or reload malformed persisted governance state before retrying; snapshot identities, revisions, candidate states, streak details, and private causes remain private. |
| `EXECUTION_HEAL_ACCEPTED_FACT_INVALID` | `INVALID_ARGUMENT` | `accepted heal fact is invalid` | none | Correct the accepted fact before retrying; fact kinds, provenance, payloads, decision bands, and identities remain private. |
| `EXECUTION_HEAL_TERMINAL_EFFECT_CONFLICT` | `CONFLICT` | `heal terminal effect conflicts with persisted state` | none | Re-read and reconcile the persisted terminal effect; effect kinds, candidate hashes, bands, contributions, and private causes remain private. |

## Automation

| Code | Kind | Safe message | Allowed params / violations | Notes |
|---|---|---|---|---|
| `AUTOMATION_EXECUTION_FLOW_INVALID` | `INVALID_ARGUMENT` | `execution flow input is invalid` | ordered typed violations only | Aggregate validation envelope: one top-level fault carrying every field failure as an ordered violation. Never one code per failing field, never a joined message. Covers the execution flow, its versions, and their items; version failures reaching it through the aggregate propagate unwrapped rather than nesting. Violation order follows the caller's declaration order with 0-based indexes, and is capped per the "Violation codes" rules. |
| `AUTOMATION_FOLDER_NOT_FOUND` | `NOT_FOUND` | `automation folder was not found` | none | Do not expose authorization-sensitive detail. |
| `AUTOMATION_FOLDER_INVALID` | `INVALID_ARGUMENT` | `automation folder is invalid` | none | Correct malformed folder identities, names, kinds, or occupancy before retrying; rejected values remain private. |
| `AUTOMATION_FOLDER_TREE_INVALID` | `FAILED_PRECONDITION` | `automation folder tree is invalid` | none | Repair duplicate, orphaned, mixed-kind, cyclic, or over-depth persisted hierarchy state; folder identities and names remain private. |
| `AUTOMATION_FOLDER_NOT_EMPTY` | `FAILED_PRECONDITION` | `automation folder must be empty` | none | Remove child folders and assets before deletion; occupancy and folder identities remain private. |
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
| `AUTOMATION_SAMPLING_PUBLICATION_DIGEST_MISMATCH` | `INVALID_ARGUMENT` | `sampling publication digest does not match the request payload` | none | Reject before any sampling-publication transaction operation; request digests, publication identities, and payload values remain private. |
| `AUTOMATION_SAMPLING_PUBLICATION_UNAVAILABLE` | `UNAVAILABLE` | `sampling publication service is unavailable` | none | Supply a valid transaction dependency before retrying; dependency details remain private. |
| `AUTOMATION_SAMPLING_PUBLICATION_ADAPTER_CONTRACT_VIOLATION` | `INTERNAL` | `sampling publication adapter returned an invalid outcome` | none | The adapter outcome is malformed; preserve its cause only for diagnostics and do not expose outcome, identity, digest, or payload details. |
| `AUTOMATION_SAMPLING_PUBLICATION_AUTHORITY_CONFLICT` | `CONFLICT` | `sampling publication authority changed before the publication could be applied` | none | Re-read node authority before retrying; conflict preserves rollback and does not disclose publication or revision details. |

## Sampling, evidence, fingerprint, interpolation, parameter

| Code | Kind | Safe message | Allowed params / violations | Notes |
|---|---|---|---|---|
| `SAMPLING_PUBLICATION_IDENTITY_CONFLICT` | `CONFLICT` | `sampling publication identity conflicts with an existing request` | none | A replay with the same publication identity but a different request digest is rejected without exposing identity or digest values. Produced from `application/automation`; the `SAMPLING_*` prefix is authoritative per `v0.5-error-inventory.md:37` — producer package and code prefix are intentionally not aligned. |
| `SAMPLING_SESSION_INPUT_INVALID` | `INVALID_ARGUMENT` | `sampling session input is invalid` | ordered typed violations only | Aggregate envelope for session construction input. A start-url parse failure is kept only as a private cause, because `url.Error` formats the whole URL — including path and query — into its own text. |
| `SAMPLING_SESSION_STATE_INVALID` | `FAILED_PRECONDITION` | `sampling session state does not allow this operation` | none | The lifecycle state machine rejected the transition, or a capture arrived outside the recording state. Remediation is to reach a legal state first. The current and requested statuses stay private. |
| `SAMPLING_CAPTURE_INVALID` | `INVALID_ARGUMENT` | `sampling capture is invalid` | ordered typed violations only | Aggregate envelope for capture input. A captured element target spec that fails its own validation propagates `FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID` unwrapped rather than nesting inside this code. Action kinds and captured values stay private. |
| `SAMPLING_DRAFT_INVALID` | `INVALID_ARGUMENT` | `sampling draft is invalid` | ordered typed violations only | Aggregate envelope for draft structure and identity: blank or duplicate ids, non-permutation reorders, and impossible container selections. Step and element target ids stay private. Step-identity violations use an unindexed `steps.*` path because the draft walk descends into children and validation branches without carrying a path. |
| `SAMPLING_DRAFT_STEP_NOT_FOUND` | `NOT_FOUND` | `sampling draft step was not found` | none | Distinct from the element target code because the frontend guidance differs; using one code with a subject parameter would push that branch to the frontend. |
| `SAMPLING_DRAFT_NODE_NOT_FOUND` | `NOT_FOUND` | `unpublished element target was not found` | none | The element target does not exist in the draft. |
| `SAMPLING_DRAFT_NODE_IN_USE` | `FAILED_PRECONDITION` | `unpublished element target is still referenced` | none | `FAILED_PRECONDITION` rather than `NOT_FOUND`: the element target exists, and remediation is to remove the referencing steps first. |
| `SAMPLING_DRAFT_INDEX_OUT_OF_RANGE` | `OUT_OF_RANGE` | `sampling draft index is out of range` | none | Separate from the draft envelope because remediation differs: supply a legal index rather than repair the draft structure. |
| `SAMPLING_PUBLICATION_MAPPING_INVALID` | `INVALID_ARGUMENT` | `sampling publication mapping is invalid` | ordered typed violations only | Aggregate envelope for the temporary-to-formal element target mapping set. Temporary and formal identities stay private. `application/automation` propagates this code unwrapped instead of adding an uncoded outer layer. |
| `SAMPLING_WORKSPACE_INVALID` | `INVALID_ARGUMENT` | `sampling workspace is invalid` | ordered typed violations only | Aggregate envelope for rebuilding the element-target-to-step projection. Step and temporary element target ids stay private. |
| `SAMPLING_INTERNAL` | `INTERNAL` | `sampling operation could not be completed` | none | A nil receiver (caller code defect) or a UUID entropy-source failure. Neither has a caller remediation, so both converge on one code rather than adding i18n keys nobody can act on; the entropy cause stays private. |
| `EVIDENCE_STEP_TRANSITION_COMMIT_INVALID` | `INVALID_ARGUMENT` | `step transition commit is invalid` | ordered typed violations only | Aggregate envelope for the commit, its terminal event, and its four fact collections. Sub-validation failures of contained observations and groups degrade into this envelope's violations rather than nesting, so the host classifies without recursive unwrapping. No commit, execution, step, validation, heal, group, or element target identity reaches public text. |
| `EVIDENCE_COMMIT_FACT_LIMIT_EXCEEDED` | `OUT_OF_RANGE` | `step transition commit exceeds its fact limit` | none | Separate from the commit envelope because the remediation differs: split the commit rather than correct a field. Checked before any other commit rule, because it also bounds the violation walks — a maximum-size commit must not be walked in full only to be rejected for being too large. |
| `EVIDENCE_STEP_PROGRESS_EVENT_INVALID` | `INVALID_ARGUMENT` | `step progress event is invalid` | ordered typed violations only | Aggregate envelope for non-terminal progress events. The rejected phase is not echoed. |
| `EVIDENCE_STEP_FACT_INVALID` | `INVALID_ARGUMENT` | `step fact is invalid` | ordered typed violations only | Aggregate envelope for terminal step facts. Phase is a closed set, so a non-terminal value is either a known non-terminal state or arbitrary caller input; either way the caller can read its own phase back from the fact, so it is not echoed. |
| `EVIDENCE_HEAL_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `heal observation is invalid` | ordered typed violations only | Aggregate envelope shared by heal observation validation and the standalone decision-band and confidence validators. The candidate hash identifies a heal candidate and the decision band is caller-declared; neither is echoed. |
| `EVIDENCE_VALIDATION_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `validation observation is invalid` | ordered typed violations only | Aggregate envelope shared by validation value, branch disposition, progress observation, and final observation validation. Expected and actual values are observed page content, so their sub-validation failures degrade into violations here rather than nesting a fault whose text could carry that content. |
| `EVIDENCE_VALIDATION_GROUP_OBSERVATION_INVALID` | `INVALID_ARGUMENT` | `validation group observation is invalid` | ordered typed violations only | Aggregate envelope for group terminal observations and their expected member list. Branch, group, and element target identities and the terminal reason stay private. |
| `FINGERPRINT_SELECTOR_INVALID` | `INVALID_ARGUMENT` | `element selector is invalid` | none | Selector source values are never exposed in the public message. |
| `FINGERPRINT_ELEMENT_TARGET_SPEC_INVALID` | `INVALID_ARGUMENT` | `element target spec is invalid` | ordered typed violations only | Aggregate validation envelope for the spec, its selectors, and its embedded fingerprint descriptor. Sub-validation failures degrade into this envelope's violations rather than nesting a second fault, so the host classifies without recursive unwrapping. Selector values, UUIDs, and identities remain private. |
| `FINGERPRINT_DESCRIPTOR_INVALID` | `INVALID_ARGUMENT` | `element fingerprint descriptor is invalid` | ordered typed violations only | Aggregate validation envelope for a descriptor validated on its own. It is deliberately separate from the element target spec code because `domain/heal` validates descriptors directly without going through a spec; folding it in would leave that path unclassified. Tag, attribute, and framework values remain private. |
| `FINGERPRINT_FRAMEWORK_STACK_INVALID` | `INVALID_ARGUMENT` | `framework stack is invalid` | ordered typed violations only | Aggregate validation envelope shared by single framework info, framework stack, and detector-set validation. Framework kinds and evidence kinds are closed sets, so a rejected value is by definition arbitrary caller input and never enters public text. |
| `FINGERPRINT_FRAMEWORK_DETECTOR_FAILED` | `INTERNAL` | `framework detection could not be completed` | none | A host-supplied detector failed. Its error text is outside Core's control and may carry page URLs or DOM fragments, so it is retained only as a private cause reachable through `Unwrap`. Not an invalid argument: the caller has no runtime remediation. |
| `PARAMETER_NAME_INVALID` | `INVALID_ARGUMENT` | `parameter name is invalid` | none | Covers blank, non-UTF-8, over-limit, and control/format-character names. The rejected name is caller input and never appears in public text. |
| `PARAMETER_VALUE_INVALID` | `INVALID_ARGUMENT` | `parameter value is invalid` | none | Covers non-canonical numbers, malformed number input, and over-size text, number, and multi-select payloads. Over-size shares this code rather than getting `OUT_OF_RANGE` because the remediation is identical: supply a different value. Neither the value nor its type is echoed — an unsupported type is by definition outside the closed set, so it is arbitrary caller input, and the caller can read its own type back through `Type()`. Malformed number input keeps a value-free private cause. |
| `PARAMETER_CONSTRAINT_UNSATISFIED` | `INVALID_ARGUMENT` | `parameter value does not satisfy its constraint` | none | Covers unsupported constraint types, type mismatch, and select-option membership. Constraint type, value type, and option values stay private; the caller supplied all three. A value that fails its own validation keeps `PARAMETER_VALUE_INVALID` instead of being re-labelled, so the host is not forced to unwrap to learn the value itself was malformed. |
| `PARAMETER_BINDING_UNRESOLVABLE` | `FAILED_PRECONDITION` | `parameter binding cannot be resolved` | none | `FAILED_PRECONDITION` rather than `INVALID_ARGUMENT`: a missing parent parameter means the surrounding scope has not supplied a required value, so the caller must supply context, not a different argument. The parent parameter name is never echoed — it is caller-declared and the parent scope map is the caller's own. |
| `INTERPOLATION_RESOLVER_REQUIRED` | `FAILED_PRECONDITION` | `variable resolver is required` | none | A resolver is needed only when interpolation syntax is present. |
| `INTERPOLATION_EXPRESSION_INVALID` | `INVALID_ARGUMENT` | `variable expression is invalid` | none | Source expression and variable name never enter the public message. |
| `INTERPOLATION_VARIABLE_UNDEFINED` | `NOT_FOUND` | `referenced variable is not defined` | none | Variable name and source expression remain private. |

These families are added only in the atomic bounded-context migration that introduces the corresponding producer. Every addition must include its `Kind`, fixed safe fallback message, allowed parameter/violation schema, retry behavior, public consumer, and any persistence mapping.

## Contexts that deliberately own no code family

### `domain/heal`

`domain/heal` has no `HEAL_*` family, and its remaining internal bare errors are permitted rather than pending. The rule is the same one that sizes every other family: only a business failure that crosses the Core public boundary needs a registered code, and an internal invariant may stay an ordinary Go error.

Verified reachability of its exported error surface:

| Exported | Callers outside `domain/heal` |
|---|---|
| `Assess` | 2, both in `domain/node` (`step.go`, `validation.go`) |
| `Decision.Validate` | `domain/node` only (`step.go`, `validation.go`) |
| `NewDefaultHealerWithPolicy` | none |
| `ValidateSamples` | none |
| `Thresholds.Validate`, `Weights.Validate` | none outside the package |

Nothing reaches a host except through `domain/node`, which owns the `EXECUTION_*` family. Classification therefore belongs at the `domain/node` boundary, not to a family of its own — adding one would mean two codes for one failure and would push heal-internal vocabulary into the published contract.

Two conditions keep that true, and both are load-bearing:

- **`domain/node` must classify at its boundary.** Today `step.go` and `validation.go` wrap heal failures as `fmt.Errorf("invalid heal decision: %w", err)` and `fmt.Errorf("assess heal decision: %w", err)` — uncoded wrappers, so a heal failure currently crosses the public boundary with no code at all. Until those four sites carry `EXECUTION_*` codes, the absence of a heal family is a gap rather than a decision.
- **Heal text must stay free of observed values.** `domain/heal` echoes no selector, page URL, origin, or fingerprint value in any error. The single dynamic value it formats is `policy.MinimumMargin` (`assessment.go:45`), a caller-supplied configuration float rather than observed page content or user input.

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
