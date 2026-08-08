package execution

import (
	"github.com/Capsule7446/healix-core/domain/fault"
)

// WorkerFence 标识唯一获准修改一个 Instance 的宿主 worker。
// ClaimToken 是宿主生成的 opaque 值；每次成功领取（包括同一 worker 重新领取）都必须
// 全局唯一，避免旧 owner 因 ABA token 复用再次通过栅栏。
type WorkerFence struct {
	InstanceID InstanceID
	ClaimToken string
}

// Validate 校验 worker fence 的实例身份和非空领取令牌。
func (f WorkerFence) Validate() error {
	if f.InstanceID.Validate() != nil || f.ClaimToken == "" {
		return mustExecutionFault(fault.InvalidArgument, CodeWorkerFenceInvalid, "worker execution authority is invalid")
	}
	return nil
}

// NewStaleWorkerFenceError 构造表示 worker 权限已过期的冲突错误。
func NewStaleWorkerFenceError() error {
	return mustExecutionFault(fault.Conflict, CodeWorkerFenceStale, "worker execution authority is stale")
}
