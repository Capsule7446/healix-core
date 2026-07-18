package contract_test

import (
	"context"
	"testing"

	coreengine "github.com/Capsule7446/healix-core/application/engine"
	"github.com/Capsule7446/healix-core/domain/fingerprint"
	"github.com/Capsule7446/healix-core/domain/heal"
	"github.com/Capsule7446/healix-core/domain/interpolation"
	"github.com/Capsule7446/healix-core/domain/metrics"
	"github.com/Capsule7446/healix-core/domain/node"
	"github.com/Capsule7446/healix-core/domain/sampling"
	"github.com/Capsule7446/healix-core/domain/workspace"
)

type consumerResolver map[string]string

func (r consumerResolver) Variable(name string) (string, bool) {
	value, ok := r[name]
	return value, ok
}

type consumerDriver struct{}

func (consumerDriver) Navigate(context.Context, string) error { return nil }
func (consumerDriver) Press(context.Context, string) error    { return nil }
func (consumerDriver) Locate(context.Context, fingerprint.NodeSpec) (node.Element, error) {
	return nil, node.ErrElementNotFound
}
func (consumerDriver) Snapshot(context.Context) (heal.DOMSnapshot, error) {
	return consumerSnapshot{}, nil
}
func (consumerDriver) WaitNetworkIdle(context.Context) error { return nil }

type consumerSnapshot struct{}

func (consumerSnapshot) Candidates(context.Context) ([]heal.SnapshotCandidate, error) {
	return nil, nil
}

func TestPublicConsumerCanUseCoreContracts(t *testing.T) {
	value, err := interpolation.Expand("${tenant}", consumerResolver{"tenant": "north"})
	if err != nil || value != "north" {
		t.Fatalf("public interpolation contract = %q, %v", value, err)
	}
	if err := coreengine.RunProgram(context.Background(), node.Program{}, coreengine.Config{
		RunID: "consumer-run", Driver: consumerDriver{}, Healer: heal.NewDefaultHealer(),
	}); err == nil {
		t.Fatal("public RunProgram accepted a missing root")
	}
	_ = coreengine.CompiledExecution{}
	_ = metrics.Query{}
	_ = sampling.MatchProfile{}
	_ = workspace.TestTaskRunPlan{}
}

var _ interpolation.Resolver = consumerResolver{}
var _ node.Driver = consumerDriver{}
