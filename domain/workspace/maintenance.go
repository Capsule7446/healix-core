package workspace

type StorageStats struct {
	DatabaseBytes          int64
	RecordingBytes         int64
	NetworkBodyBytes       int64
	ExecutionResourceBytes int64
	OldestExecutionAt      int64
	RecordingCount         int
	NetworkRequestCount    int
	RunCount               int
	NodeCount              int
	WorkflowCount          int
	EnvironmentCount       int
}

type CleanupPreview struct {
	RunCount         int
	ResourceBytes    int64
	OldestFinishedAt int64
}
