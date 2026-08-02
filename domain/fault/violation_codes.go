package fault

// Violation reason codes form the closed vocabulary shared by every context's
// aggregate validation envelope.
//
// A violation answers two separate questions: its field carries *which* input
// failed, and its code carries *why* it failed. Keeping the "why" vocabulary
// small and kernel-owned is what stops each context from minting a near
// identical code per failing field, which would make the frontend i18n key
// granularity unmanageable while saying nothing new.
//
// These codes are registered under "Violation codes" in the error-code registry.
// They are deliberately not context-prefixed because they are owned by the
// shared kernel rather than by any bounded context, but they obey the same
// immutability rules as top-level codes: never renamed, never reused, and
// tombstoned rather than deleted.
//
// They are reason codes only. They must never appear as the code of a top-level
// Error, whose code names the aggregate that rejected the input.
const (
	// CodeFieldRequired reports an absent or blank input that is mandatory.
	CodeFieldRequired Code = "VALIDATION_FIELD_REQUIRED"
	// CodeFieldInvalid reports a present input whose value is not acceptable.
	CodeFieldInvalid Code = "VALIDATION_FIELD_INVALID"
	// CodeFieldDuplicate reports an input that repeats a value required to be unique.
	CodeFieldDuplicate Code = "VALIDATION_FIELD_DUPLICATE"
	// CodeFieldMismatch reports an input that contradicts the aggregate holding it.
	CodeFieldMismatch Code = "VALIDATION_FIELD_MISMATCH"
)
