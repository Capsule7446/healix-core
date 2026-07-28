package engine

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Capsule7446/healix-core/domain/execution"
	"github.com/Capsule7446/healix-core/domain/node"
)

func TestCompileSnapshotDraftRejectsDuplicatePublicIdentities(t *testing.T) {
	base := minimalCompilerPlan()
	tests := []struct {
		name   string
		mutate func(*execution.Draft)
		want   string
	}{
		{"workflow version", func(d *execution.Draft) { d.Workflows = append(d.Workflows, d.Workflows[0]) }, "duplicate workflow version"},
		{"node dependency", func(d *execution.Draft) {
			nodeSnapshot := compilerNodeSnapshot(compilerNodeV1, "button")
			d.Nodes = []execution.NodeSnapshot{nodeSnapshot, nodeSnapshot}
		}, "duplicate node dependency"},
		{"execution", func(d *execution.Draft) { d.Entries = append(d.Entries, d.Entries[0]) }, "duplicate execution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := base
			tt.mutate(&draft)
			snapshot, err := runSnapshotForCompilerTest(base, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = compileSnapshotDraft(draft, snapshot)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunProgramRejectsNilContextAndNegativeInterval(t *testing.T) {
	program := node.Program{Root: &runtimeCaptureNode{}}
	tests := []struct {
		name     string
		ctx      context.Context
		interval time.Duration
		want     string
	}{
		{"nil context", nil, 0, "context is required"},
		{"negative interval", context.Background(), -time.Nanosecond, "step interval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runProgram(tt.ctx, program, Config{RunID: "run", Driver: &engineTestDriver{}, StepInterval: tt.interval})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestMillisecondsDurationRejectsOnlyInvalidInt64Boundaries(t *testing.T) {
	const maxMilliseconds = int64(^uint64(0)>>1) / int64(time.Millisecond)
	tests := []struct {
		name         string
		milliseconds int64
		want         time.Duration
		wantError    bool
	}{
		{"negative one", -1, 0, true},
		{"zero", 0, 0, false},
		{"one", 1, time.Millisecond, false},
		{"one below maximum", maxMilliseconds - 1, time.Duration(maxMilliseconds-1) * time.Millisecond, false},
		{"maximum", maxMilliseconds, time.Duration(maxMilliseconds) * time.Millisecond, false},
		{"one above maximum", maxMilliseconds + 1, 0, true},
		{"maximum int64", math.MaxInt64, 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := millisecondsDuration(test.milliseconds)
			if (err != nil) != test.wantError {
				t.Fatalf("millisecondsDuration(%d) error = %v, wantError = %t", test.milliseconds, err, test.wantError)
			}
			if got != test.want {
				t.Fatalf("millisecondsDuration(%d) = %s, want %s", test.milliseconds, got, test.want)
			}
		})
	}
}

func TestCompilerRejectsNegativeWaitMilliseconds(t *testing.T) {
	base := minimalCompilerPlan()
	snapshot, err := runSnapshotForCompilerTest(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	draft := base
	draft.Workflows[0].Steps = []execution.Step{{ID: "wait", DisplayName: "wait", Kind: execution.WaitStep, WaitKind: "sleep", WaitMS: -1}}
	_, err = compileSnapshotDraft(draft, snapshot)
	if err == nil || !strings.Contains(err.Error(), "wait step wait") || !strings.Contains(err.Error(), "duration milliseconds -1") {
		t.Fatalf("error = %v, want contextual duration validation", err)
	}
}
