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
| `EXECUTION_PLAN_UNSEALED` | `FAILED_PRECONDITION` | `execution plan must be sealed` | none | Not retryable without sealing. |
| `EXECUTION_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `execution status transition is invalid` | none | The lifecycle state is not itself a fault. |
| `EXECUTION_RUN_STATUS_TRANSITION_INVALID` | `FAILED_PRECONDITION` | `run status transition is invalid` | none | The containing run lifecycle is not itself a fault. |
| `EXECUTION_WORKER_FENCE_STALE` | `CONFLICT` | `worker execution authority is stale` | none | Re-read/claim authority; no raw fence value. |
| `EXECUTION_ENTRY_STATES_INVALID` | `FAILED_PRECONDITION` | `execution entry states are invalid` | none | Scheduling state must match the sealed run plan and serial lifecycle constraints. |
| `EXECUTION_FACT_COMMITTER_REQUIRED` | `FAILED_PRECONDITION` | `execution fact committer is required` | none | The caller must provide both transaction and governance dependencies. |
| `EXECUTION_AUTHORITY_VERIFIER_REQUIRED` | `FAILED_PRECONDITION` | `execution authority verifier is required` | none | Execution requires an authority verifier before it can run side effects. |
| `EXECUTION_IDENTITY_MISMATCH` | `FAILED_PRECONDITION` | `execution identity does not match the sealed entry` | none | Rebuild execution configuration from the authoritative sealed entry; no identity/token values are exposed. |

## Automation

| Code | Kind | Safe message | Allowed params / violations | Notes |
|---|---|---|---|---|
| `AUTOMATION_TEST_TASK_INVALID` | `INVALID_ARGUMENT` | `test task input is invalid` | ordered typed violations only | Aggregate validation envelope. |
| `AUTOMATION_FOLDER_NOT_FOUND` | `NOT_FOUND` | `automation folder was not found` | none | Do not expose authorization-sensitive detail. |
| `AUTOMATION_AGGREGATE_DELETED` | `FAILED_PRECONDITION` | `automation aggregate has been deleted` | none | Mutations require a restored aggregate; no aggregate identity is exposed. |
| `AUTOMATION_VERSION_NUMBER_EXHAUSTED` | `RESOURCE_EXHAUSTED` | `automation version number capacity is exhausted` | none | Publishing cannot continue until version capacity is remediated. |
| `AUTOMATION_CONFIGURATION_INVALID` | `FAILED_PRECONDITION` | `automation service is not configured` | none | The caller must provide valid service dependencies; no dependency identity is exposed. |
| `AUTOMATION_HEAL_CANDIDATE_STALE_BASE` | `CONFLICT` | `heal candidate base version is no longer current` | none | Refresh the candidate and current element target before retrying; no candidate/version identifiers are exposed. |
| `SAMPLING_PUBLICATION_IDENTITY_CONFLICT` | `CONFLICT` | `sampling publication identity conflicts with an existing request` | none | A replay with the same publication identity but a different request digest is rejected without exposing identity or digest values. |

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
