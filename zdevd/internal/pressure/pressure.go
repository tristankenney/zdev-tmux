// Package pressure ranks non-demanding work that is quietly getting worse.
// Pressure changes what gets a hearing at a boundary; it never grants a
// notification or any other right to interrupt.
package pressure

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

const (
	KindDue       = "due"
	KindRot       = "rot"
	KindAckedWait = "acked-wait"
	visibleAt     = 25
)

// Item is one pressure-promoted candidate. Score is ordinal (0..100), not a
// duration estimate: consumers may compare it but must not present it as a
// promise about time or urgency.
type Item struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Project string `json:"project,omitempty"`
	Score   int    `json:"score"`
	AgeSec  int64  `json:"age_sec,omitempty"`
}

// Promotions derives the boundary-visible portion of the drifting register.
// Output is highest pressure first with stable identity tiebreaks.
func Promotions(s *proto.Snapshot, now int64) []Item {
	if s == nil {
		return nil
	}
	byID := map[string]Item{}
	for _, c := range s.Commitments {
		project, ok := strings.CutPrefix(c.Kind, "task:")
		if !ok || project == "" || c.At <= 0 {
			continue
		}
		score := dueScore(c.At - now)
		add(byID, Item{ID: "due:" + c.Source + ":" + c.ID, Kind: KindDue,
			Title: c.Title, Project: project, Score: score, AgeSec: now - c.At})
	}
	if s.ReviewGauge != nil {
		for _, repo := range s.ReviewGauge.Repos {
			for _, row := range repo.Rows {
				score := rotScore(row.AgeSec)
				title := fmt.Sprintf("%s · %s", reviewLabel(row.Bucket), repo.Repo)
				add(byID, Item{ID: "rot:" + row.Project, Kind: KindRot,
					Title: title, Project: row.Project, Score: score, AgeSec: row.AgeSec})
			}
		}
	}
	for _, p := range s.Projects {
		if p.Attention != proto.AttWaiting || !p.WaitAcknowledged {
			continue
		}
		age := ageSince(p.WaitStartedTS, now)
		score := ackedWaitScore(age)
		title := p.WaitSummary
		if title == "" {
			title = "acknowledged wait still unresolved"
		}
		add(byID, Item{ID: "acked-wait:" + p.Name, Kind: KindAckedWait,
			Title: title, Project: p.Name, Score: score, AgeSec: age})
	}

	out := make([]Item, 0, len(byID))
	for _, item := range byID {
		if item.Score >= visibleAt {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func add(dst map[string]Item, item Item) {
	if old, ok := dst[item.ID]; !ok || item.Score > old.Score {
		dst[item.ID] = item
	}
}

// Due pressure accelerates into the deadline. Overdue work tops a boundary,
// but does not become a notification here.
func dueScore(until int64) int {
	switch {
	case until <= 0:
		return 100
	case until <= 8*60*60:
		return 75 + int((8*60*60-until)*24/(8*60*60))
	case until <= 24*60*60:
		return 50 + int((24*60*60-until)*24/(16*60*60))
	case until <= 48*60*60:
		return 25 + int((48*60*60-until)*24/(24*60*60))
	default:
		return 0
	}
}

// Review rot grows linearly from first visibility at one day to maximum at
// seven days. Unknown age carries no pressure.
func rotScore(age int64) int {
	if age < 24*60*60 {
		return 0
	}
	if age >= 7*24*60*60 {
		return 100
	}
	return 25 + int((age-24*60*60)*75/(6*24*60*60))
}

// An acknowledged wait has already been seen, so it starts visible at the
// next boundary and rises with unresolved age instead of interrupting again.
func ackedWaitScore(age int64) int {
	if age < 0 {
		age = 0
	}
	score := 25 + int(age/(15*60))*5
	if score > 100 {
		return 100
	}
	return score
}

func ageSince(ts, now int64) int64 {
	if ts <= 0 || now <= ts {
		return 0
	}
	return now - ts
}

func reviewLabel(bucket string) string {
	switch bucket {
	case proto.ReviewBucketReady:
		return "ready to land"
	case proto.ReviewBucketNeedsFix:
		return "review needs a fix"
	case proto.ReviewBucketWillRot:
		return "uncommitted work will rot"
	default:
		return "review debt"
	}
}
