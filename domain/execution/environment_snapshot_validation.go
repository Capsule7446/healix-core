package execution

import (
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"

	"github.com/Capsule7446/healix-core/domain/weburl"
)

// validateEnvironmentSnapshot builds one aggregate violation envelope for the
// instance's environment, screenshot, and healer policy snapshots. Every sub-check
// stays an ordinary Go error and is discarded once recorded as a generic
// violation, so an environment property or variable name — caller-declared,
// and never useful to echo back — never reaches public text. Property and
// variable maps are walked in sorted key order so violation order is a
// function of the input alone.
func validateEnvironmentSnapshot(schemaVersion InstanceSnapshotSchema, env EnvironmentSnapshot, screenshot ScreenshotPolicySnapshot, healer HealerPolicySnapshot) error {
	var violations []fault.Violation
	violations = appendEnvironmentIdentityViolations(violations, env)
	violations = appendEnvironmentVariableViolations(violations, schemaVersion, env)
	violations = appendScreenshotPolicyViolations(violations, screenshot)
	violations = appendHealerPolicyViolations(violations, healer)
	if len(violations) != 0 {
		return environmentSnapshotInvalidError(violations)
	}
	return nil
}

func appendEnvironmentIdentityViolations(violations []fault.Violation, v EnvironmentSnapshot) []fault.Violation {
	if atCap(violations) {
		return violations
	}
	if v.Revision == 0 {
		violations = append(violations, mustViolation(fault.CodeFieldRequired, "environment.revision", "environment revision is required"))
	}
	if !validString(v.ID, true) || !validString(v.DisplayName, true) || !validString(v.BaseURL, false) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.identity", "environment identity is invalid"))
	}
	// The shared rule also rejects control characters, which this call site
	// previously did not check even though the two navigation call sites did.
	// A base URL is concatenated into request targets, so a raw CR here is
	// the same splitting vector it is there.
	if v.BaseURL != "" && !weburl.Accept(v.BaseURL) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.baseUrl", "environment base URL must be an absolute HTTP(S) URL without credentials"))
	}
	return violations
}

func appendEnvironmentVariableViolations(violations []fault.Violation, schemaVersion InstanceSnapshotSchema, v EnvironmentSnapshot) []fault.Violation {
	if schemaVersion == InstanceSnapshotSchemaV1 {
		if len(v.Variables) != 0 {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.variables", "a V1 environment snapshot cannot contain typed variables"))
		}
		for _, name := range sortedKeys(v.Properties) {
			if atCap(violations) {
				return violations
			}
			value := v.Properties[name]
			if parameter.ValidateName(name) != nil || !validString(value, false) {
				violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.properties", "an environment property is invalid"))
			}
		}
		return violations
	}
	if len(v.Properties) != 0 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.properties", "a V2 environment snapshot cannot contain legacy properties"))
	}
	for _, name := range sortedKeys(v.Variables) {
		if atCap(violations) {
			return violations
		}
		value := v.Variables[name]
		if err := parameter.ValidateName(name); err != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.variables", "an environment variable name is invalid"))
			continue
		}
		if err := value.Validate(); err != nil {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.variables", "an environment variable value is invalid"))
		}
	}
	return violations
}

func appendScreenshotPolicyViolations(violations []fault.Violation, v ScreenshotPolicySnapshot) []fault.Violation {
	if atCap(violations) {
		return violations
	}
	if v.Version != ScreenshotPolicyV1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.screenshotPolicy", "screenshot policy version is unsupported"))
	}
	if !validString(v.Destination, false) || (v.Enabled && strings.TrimSpace(v.Destination) == "") {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.screenshotPolicy", "screenshot policy destination is invalid"))
	}
	return violations
}

func appendHealerPolicyViolations(violations []fault.Violation, v HealerPolicySnapshot) []fault.Violation {
	if atCap(violations) {
		return violations
	}
	if v.Version != HealerPolicyV1 {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.healerPolicy", "healer policy version is unsupported"))
	}
	if !unit(v.ReviewCap) || !unit(v.AppliedCap) || v.ReviewCap >= v.AppliedCap {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.healerPolicy", "healer policy thresholds are invalid"))
	}
	weights := []float64{v.Weights.Tag, v.Weights.ID, v.Weights.RoleName, v.Weights.Class, v.Weights.Attrs, v.Weights.Text, v.Weights.Index, v.Weights.Neighbor, v.Weights.LabelText, v.Weights.Container}
	total := 0.0
	for _, x := range weights {
		if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 {
			violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.healerPolicy", "a healer weight is invalid"))
			return violations
		}
		total += x
	}
	if total == 0 || math.IsInf(total, 0) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.healerPolicy", "healer weights require a positive finite total"))
	}
	return violations
}

func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
