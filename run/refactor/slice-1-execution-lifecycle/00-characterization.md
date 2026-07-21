# Slice 1 — 00 Characterization

## Scope

Execution lifecycle composition in `application/engine.RunProgram`.

## Existing behavior captured

- Empty RunID returns an error before Driver use.
- Nil Driver returns an error before program execution.
- Nil Program.Root returns an error before runtime execution.
- Variables are copied into a fresh runtime scratchpad; later caller mutation does not alter the run snapshot.
- Recorder.Start runs before root execution.
- Recorder.Stop runs after root execution, with a detached five-second cleanup timeout and retain=true.
- Root execution errors propagate unchanged unless recorder stop also fails; then errors are joined with stop context.
- Recorder start failure prevents root execution and is wrapped with start context.
- Context cancellation remains visible to root execution while recorder cleanup remains possible.

## Required characterization tests

- Existing `application/engine` unit and contract matrix tests.
- Add/verify fake root, fake recorder, and variable-isolation cases before changing implementation.
- Cover start failure, root failure, stop failure, and start/root/stop ordering.

## G2 gate

The current repository test suite is the behavior baseline. No production code has been changed by this workflow. Implementation is blocked until these cases pass against the current path and the race/vet checks complete.
