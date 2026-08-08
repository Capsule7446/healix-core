package conformancetest

import (
	"context"
	"reflect"
	"testing"
	"time"

	application "github.com/Capsule7446/healix-core/application/automation"
	"github.com/Capsule7446/healix-core/domain/fault"
)

// HealReviewFaultPoint 标识审核事务中用于验证原子回滚的故障注入位置。
type HealReviewFaultPoint string

const (
	// HealFaultAfterCandidate 在候选写入后注入故障。
	HealFaultAfterCandidate HealReviewFaultPoint = "AFTER_CANDIDATE"
	// HealFaultAfterNode 在节点写入后注入故障。
	HealFaultAfterNode HealReviewFaultPoint = "AFTER_NODE"
	// HealFaultAfterStreak 在 streak 写入后注入故障。
	HealFaultAfterStreak HealReviewFaultPoint = "AFTER_STREAK"
	// HealFaultAfterAudit 在审计记录写入后注入故障。
	HealFaultAfterAudit HealReviewFaultPoint = "AFTER_AUDIT"
	// HealFaultAfterOutbox 在 outbox 写入后注入故障。
	HealFaultAfterOutbox HealReviewFaultPoint = "AFTER_OUTBOX"
	// HealFaultBeforeReplay 在重放返回前注入故障。
	HealFaultBeforeReplay HealReviewFaultPoint = "BEFORE_REPLAY"
)

// HealReviewSnapshot 是审核 conformance fixture 保存的宿主状态快照不透明值。
type HealReviewSnapshot any

// HealReviewFixture 定义审核事务 conformance 测试所需的适配器、状态控制和断言端口。
type HealReviewFixture interface {
	application.HealReviewTransaction
	// Intent 返回待提交的审核意图。
	Intent() application.HealReviewIntent
	// CompetingIntents 返回用于并发竞争的两个审核意图。
	CompetingIntents() (application.HealReviewIntent, application.HealReviewIntent)
	// Snapshot 返回当前宿主状态快照。
	Snapshot() HealReviewSnapshot
	// SetHealFault 在指定事务阶段注入故障。
	SetHealFault(HealReviewFaultPoint)
	// MakeCandidateStale 使候选权威状态过期。
	MakeCandidateStale()
	// MakeNodeStale 使节点权威状态过期。
	MakeNodeStale()
	// MakeCurrentBaseStale 使节点当前基线版本过期。
	MakeCurrentBaseStale()
	// MakeStreakStale 使拒绝 streak 权威状态过期。
	MakeStreakStale()
	// AssertApplied 断言审核意图已完整物化到宿主状态。
	AssertApplied(application.HealReviewIntent) error
}

// HealReviewFactory 创建一个用于审核 conformance 测试的 fixture。
type HealReviewFactory func(*testing.T) HealReviewFixture

// RunHealReview 运行审核的应用、重放、身份校验、CAS、回滚和并发竞争 conformance 场景。
func RunHealReview(t *testing.T, factory HealReviewFactory) {
	t.Helper()
	t.Run("apply-replay-digest-and-identity", func(t *testing.T) {
		f, intent := factory(t), application.HealReviewIntent{}
		intent = f.Intent()
		want := healOutcome(intent, application.HealReviewApplied)
		got, err := f.CommitHealReview(context.Background(), intent)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("apply = %#v, %v; want %#v", got, err, want)
		}
		if err := f.AssertApplied(intent); err != nil {
			t.Fatal(err)
		}
		after := f.Snapshot()
		got, err = f.CommitHealReview(context.Background(), intent)
		want.Status = application.HealReviewReplayed
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("replay = %#v, %v", got, err)
		}
		if !reflect.DeepEqual(after, f.Snapshot()) {
			t.Fatal("replay changed state")
		}
		bad := intent
		bad.RequestDigest = "sha256:bad"
		if _, err := f.CommitHealReview(context.Background(), bad); !fault.IsCode(err, application.CodeHealReviewIdentityConflict) {
			t.Fatalf("malformed digest = %v", err)
		}
		changed := intent
		changed.NextCandidate.PageURL = "/different-payload"
		if _, err := f.CommitHealReview(context.Background(), changed); !fault.IsCode(err, application.CodeHealReviewIdentityConflict) {
			t.Fatalf("payload mismatch = %v", err)
		}
		changed.RequestDigest = mustHealDigest(t, changed)
		if _, err := f.CommitHealReview(context.Background(), changed); !fault.IsCode(err, application.CodeHealReviewIdentityConflict) {
			t.Fatalf("identity conflict = %v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(HealReviewFixture)
	}{
		{"candidate", func(f HealReviewFixture) { f.MakeCandidateStale() }},
		{"node", func(f HealReviewFixture) { f.MakeNodeStale() }},
		{"current-base", func(f HealReviewFixture) { f.MakeCurrentBaseStale() }},
		{"streak", func(f HealReviewFixture) { f.MakeStreakStale() }},
	} {
		t.Run("cas-"+tc.name, func(t *testing.T) {
			f := factory(t)
			tc.mutate(f)
			before := f.Snapshot()
			if _, err := f.CommitHealReview(context.Background(), f.Intent()); !fault.IsCode(err, application.CodeHealReviewAuthorityConflict) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(before, f.Snapshot()) {
				t.Fatal("CAS failure changed state")
			}
		})
	}

	for _, point := range []HealReviewFaultPoint{HealFaultAfterCandidate, HealFaultAfterNode, HealFaultAfterStreak, HealFaultAfterAudit, HealFaultAfterOutbox, HealFaultBeforeReplay} {
		t.Run("rollback-"+string(point), func(t *testing.T) {
			f := factory(t)
			before := f.Snapshot()
			f.SetHealFault(point)
			if _, err := f.CommitHealReview(context.Background(), f.Intent()); err == nil {
				t.Fatal("fault succeeded")
			}
			if !reflect.DeepEqual(before, f.Snapshot()) {
				t.Fatal("fault changed state")
			}
		})
	}

	t.Run("concurrent-equal", func(t *testing.T) {
		f := factory(t)
		intent := f.Intent()
		results := concurrentHeal(t, f, intent, intent)
		applied, replayed := 0, 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			switch r.outcome.Status {
			case application.HealReviewApplied:
				applied++
			case application.HealReviewReplayed:
				replayed++
			}
		}
		if applied != 1 || replayed != 1 {
			t.Fatalf("applied=%d replayed=%d", applied, replayed)
		}
	})
	t.Run("concurrent-competing-decisions", func(t *testing.T) {
		f := factory(t)
		left, right := f.CompetingIntents()
		results := concurrentHeal(t, f, left, right)
		winners, losers := 0, 0
		for _, r := range results {
			if r.err == nil {
				winners++
				continue
			}
			if fault.IsCode(r.err, application.CodeHealReviewDecisionConflict) || fault.IsCode(r.err, application.CodeHealReviewAuthorityConflict) {
				losers++
				continue
			}
			t.Fatalf("loser error = %v", r.err)
		}
		if winners != 1 || losers != 1 {
			t.Fatalf("winners=%d losers=%d", winners, losers)
		}
	})
}

// healResult 保存并发审核调用的结果和错误。
type healResult struct {
	outcome application.HealReviewOutcome
	err     error
}

// concurrentHeal 并发提交多个审核意图，并收集全部结果；超时则使测试失败。
func concurrentHeal(t *testing.T, tx application.HealReviewTransaction, intents ...application.HealReviewIntent) []healResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	ch := make(chan healResult, len(intents))
	for _, intent := range intents {
		go func(i application.HealReviewIntent) {
			<-start
			o, e := tx.CommitHealReview(ctx, i)
			ch <- healResult{o, e}
		}(intent)
	}
	close(start)
	out := make([]healResult, 0, len(intents))
	for len(out) < len(intents) {
		select {
		case r := <-ch:
			out = append(out, r)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	return out
}

// healOutcome 按审核意图构造期望的事务结果。
func healOutcome(i application.HealReviewIntent, status application.HealReviewStatus) application.HealReviewOutcome {
	return application.HealReviewOutcome{Status: status, CommandID: i.CommandID, RequestDigest: i.RequestDigest, Result: application.HealReviewResult{Decision: i.Decision, Candidate: i.NextCandidate, ElementTarget: i.NextNode, Streak: i.NextStreak}}
}

// mustHealDigest 计算审核意图摘要，摘要生成失败时立即终止测试。
func mustHealDigest(t *testing.T, i application.HealReviewIntent) string {
	t.Helper()
	d, e := application.HealReviewRequestDigest(i)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
