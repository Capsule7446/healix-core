package sampling

// CaptureHandler is the application callback used by a sampling browser
// adapter. Browser lifecycle remains an application-local port; this function
// type only translates one captured domain value into its idempotent result.
type CaptureHandler func(Capture) (CaptureResult, error)
