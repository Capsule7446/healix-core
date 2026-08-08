package sampling

// CaptureHandler 定义将一次采样捕获转换为幂等结果的应用层回调。
type CaptureHandler func(Capture) (CaptureResult, error)
