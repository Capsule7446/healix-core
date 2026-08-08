package execution

import (
	"math"
	"strings"

	"github.com/Capsule7446/healix-core/domain/fault"
	"github.com/Capsule7446/healix-core/domain/parameter"

	"github.com/Capsule7446/healix-core/domain/weburl"
)

// validateEnvironmentSnapshot 为实例环境、截图和自愈策略快照构建一个聚合违规封套。各子校验
// 只产生普通 Go 错误，记录为通用违规后即丢弃，因此调用方声明的属性名或变量名不会进入
// 公开文本。属性和变量映射按排序键遍历，使违规顺序只由输入决定。
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

// appendEnvironmentIdentityViolations 追加环境修订号、身份字段和基础 URL 的违规项。
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
	// 基础 URL 会拼接进请求目标，因此必须拒绝控制字符，避免原始 CR 等字符造成请求分割。
	if v.BaseURL != "" && !weburl.Accept(v.BaseURL) {
		violations = append(violations, mustViolation(fault.CodeFieldInvalid, "environment.baseUrl", "environment base URL must be an absolute HTTP(S) URL without credentials"))
	}
	return violations
}

// appendEnvironmentVariableViolations 按快照模式校验属性或类型化变量，并按键排序遍历。
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

// appendScreenshotPolicyViolations 校验截图策略版本、启用条件和目标路径安全性。
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

// appendHealerPolicyViolations 校验自愈策略版本、阈值和权重的有限非负约束。
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
	weights := []float64{v.Weights.Tag, v.Weights.ID, v.Weights.RoleName, v.Weights.Class, v.Weights.Attrs, v.Weights.Text, v.Weights.Index, v.Weights.Neighbor, v.Weights.LabelText, v.Weights.Container, v.Weights.Framework}
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

// unit 判断浮点值是否为有限且位于闭区间 [0,1]。
func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
