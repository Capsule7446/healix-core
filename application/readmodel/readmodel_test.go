package readmodel

import (
	"reflect"
	"testing"
)

func TestReadModelsDoNotEmbedWorkspaceAggregatesOrCredentials(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(NodeListItem{}), reflect.TypeOf(WorkflowListItem{}),
		reflect.TypeOf(TestTaskListItem{}), reflect.TypeOf(RunListItem{}),
		reflect.TypeOf(ExecutionDetailView{}), reflect.TypeOf(HealingReviewView{}),
		reflect.TypeOf(DashboardView{}),
	}
	for _, typ := range types {
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if field.Name == "Password" || field.Name == "Username" {
				t.Fatalf("%s exposes credential field %s", typ.Name(), field.Name)
			}
		}
	}
}

func TestReadModelsUseStableViewSpecificFields(t *testing.T) {
	item := NodeListItem{ID: "node-1", DisplayName: "登录按钮", CurrentVersion: "version-2", VersionNumber: 2, RefCount: 3}
	if item.ID == "" || item.CurrentVersion == "" || item.VersionNumber != 2 || item.RefCount != 3 {
		t.Fatalf("unexpected node view: %+v", item)
	}
	view := HealingReviewView{ObservationID: "observation-1", Candidates: []HealingCandidateView{{CandidateHash: "candidate-1", Rank: 1, Selected: true}}}
	if len(view.Candidates) != 1 || !view.Candidates[0].Selected {
		t.Fatalf("unexpected healing view: %+v", view)
	}
}
