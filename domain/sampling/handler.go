package sampling

// CaptureHandler 是采样浏览器适配器使用的应用程序回调。浏览器生命周期仍然是应用程序本地端口；此函数类型仅将一个捕获的域值转换为其幂等结果。
type CaptureHandler func(Capture) (CaptureResult, error)
