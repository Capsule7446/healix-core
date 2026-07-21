# 07 领域交互

1. Workspace creates a run plan; compiler validates and translates it into Program/NodeSpec values.
2. Application starts a run and injects Driver, optional Healer, recorder and fact ports.
3. Node execution locates a target; on not-found it requests a snapshot and calls the healing context.
4. Healing scores candidates, returns deterministic samples, and assessment applies safety/review policy.
5. Node records the decision, re-locates the selected candidate, then installs a run-local overlay only after successful audit recording.
6. Validation nodes poll through the Driver and emit observations; sensitive actuals are masked in user-facing errors.
7. Sinks/adapters project facts into workspace evidence; the projection may be best-effort or error-propagating de待处理 on the call site.
8. Recorder lifecycle is coordinated by application engine and can contribute an error to the final run result.

Important ordering invariants: decision before overlay, successful re-location before overlay, audit before overlay, and fresh StepExecution per repeated run.
