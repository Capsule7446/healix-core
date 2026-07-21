package automation

import (
	"errors"
	"strings"
)

// ScreenshotPolicy 是通过 TestTaskRun 冻结的不可变业务意图。目的地是一个不透明的传递目标：它的路径语义、编码配置文件和验证属于外部适配器。
type ScreenshotPolicy struct {
	Enabled     bool
	Destination string
}

func NewScreenshotPolicy(enabled bool, destination string) ScreenshotPolicy {
	return ScreenshotPolicy{Enabled: enabled, Destination: strings.TrimSpace(destination)}
}

// NormalizeScreenshotPolicy 使此功能之前的历史快照保持可读。因此，零值意味着 V1 默认禁用。
func NormalizeScreenshotPolicy(policy ScreenshotPolicy) ScreenshotPolicy {
	policy.Destination = strings.TrimSpace(policy.Destination)
	return policy
}

func (p ScreenshotPolicy) Validate() error {
	p = NormalizeScreenshotPolicy(p)
	if p.Enabled && p.Destination == "" {
		return errors.New("enabled screenshot policy requires a destination")
	}
	return nil
}
