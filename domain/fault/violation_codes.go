package fault

// Violation 原因码构成所有上下文聚合校验封套共享的封闭词汇。
//
// 一项违规回答两个独立问题：field 携带失败的是“哪个”输入，code 携带失败的“原因”。
// 让“原因”词汇保持精简并由 shared kernel 拥有，可以避免各上下文为每个失败字段铸造
// 几乎相同的错误码；否则前端 i18n 键的粒度会变得不可管理，却没有增加信息。
//
// 这些码在错误码注册表的“Violation codes”项下登记。它们有意不带上下文前缀，因为
// 所有权属于 shared kernel 而非某个 bounded context；但它们遵守与顶层错误码相同的
// 不可变规则：不得重命名、复用，删除时必须墓碑化。
//
// 它们只能作为原因码使用，绝不能作为顶层 Error 的 code；顶层 code 必须命名拒绝输入
// 的聚合错误。
const (
	// CodeFieldRequired 表示必填输入缺失或为空白。
	CodeFieldRequired Code = "VALIDATION_FIELD_REQUIRED"
	// CodeFieldInvalid 表示输入存在但其值不被接受。
	CodeFieldInvalid Code = "VALIDATION_FIELD_INVALID"
	// CodeFieldDuplicate 表示输入重复了必须唯一的值。
	CodeFieldDuplicate Code = "VALIDATION_FIELD_DUPLICATE"
	// CodeFieldMismatch 表示输入与承载它的聚合相矛盾。
	CodeFieldMismatch Code = "VALIDATION_FIELD_MISMATCH"
)
