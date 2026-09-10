package pressure

import (
	"testing"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

func TestPromotionsRanksAcrossKinds(t *testing.T) {
	now := int64(1_700_000_000)
	s := &proto.Snapshot{
		Commitments: []proto.Commitment{{ID: "due", Source: "plan", Title: "ship it", Kind: "task:pay/api", At: now + 2*60*60}},
		ReviewGauge: &proto.ReviewGauge{Repos: []proto.ReviewRepo{{Repo: "pay/web", Rows: []proto.ReviewRow{{Project: "pay/web-a", Bucket: proto.ReviewBucketReady, AgeSec: 3 * 24 * 60 * 60}}}}},
		Projects:    []proto.Project{{Name: "pay/ops", Attention: proto.AttWaiting, WaitAcknowledged: true, WaitStartedTS: now - 30*60, WaitSummary: "choose a deploy"}},
	}
	got := Promotions(s, now)
	if len(got) != 3 {
		t.Fatalf("Promotions = %+v, want 3", got)
	}
	if got[0].Kind != KindDue || got[1].Kind != KindRot || got[2].Kind != KindAckedWait {
		t.Fatalf("rank order = %+v, want due, rot, acked-wait", got)
	}
}

func TestPromotionsKeepsQuietWorkBelowThreshold(t *testing.T) {
	now := int64(1_700_000_000)
	s := &proto.Snapshot{
		Commitments: []proto.Commitment{{ID: "later", Source: "plan", Title: "later", Kind: "task:pay/api", At: now + 72*60*60}},
		ReviewGauge: &proto.ReviewGauge{Repos: []proto.ReviewRepo{{Repo: "pay/web", Rows: []proto.ReviewRow{{Project: "pay/web", Bucket: proto.ReviewBucketReady, AgeSec: 2 * 60 * 60}}}}},
	}
	if got := Promotions(s, now); len(got) != 0 {
		t.Fatalf("quiet work promoted: %+v", got)
	}
}

func TestPressureCurvesAreMonotonicAndCapped(t *testing.T) {
	if !(dueScore(36*60*60) < dueScore(12*60*60) && dueScore(12*60*60) < dueScore(-1)) {
		t.Fatal("due pressure must accelerate toward and through its deadline")
	}
	if !(rotScore(24*60*60) < rotScore(4*24*60*60) && rotScore(8*24*60*60) == 100) {
		t.Fatal("rot pressure must rise linearly and cap at 100")
	}
	if ackedWaitScore(1000*60) != 100 {
		t.Fatal("acked wait pressure must cap at 100")
	}
}
