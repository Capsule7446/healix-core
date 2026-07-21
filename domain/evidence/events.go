package evidence

// StepPhaseEvent is the framework-neutral execution timeline event.
type StepPhaseEvent struct {
	ID             string
	ExecutionID    string
	WorkflowStepID string
	DisplayName    string
	Kind           string
	Phase          string
	Occurrence     int
	HierarchyPath  string
	Timestamp      int64
	ErrorMessage   string
}
