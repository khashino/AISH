package usage

import (
	"github.com/khashino/AISH/internal/provider"
	"testing"
	"time"
)

func TestEstimateAndSummary(t *testing.T) {
	u := Estimate([]provider.Message{{Role: "user", Content: "12345678"}}, "1234")
	if !u.Estimated || u.InputTokens != 2 || u.OutputTokens != 1 || u.TotalTokens != 3 {
		t.Fatalf("unexpected usage: %+v", u)
	}
	s := Summarize([]Record{{InputTokens: 2, OutputTokens: 1, TotalTokens: 3, Estimated: true, DurationMS: 1000}, {InputTokens: 4, OutputTokens: 2, TotalTokens: 6, DurationMS: 2000}})
	if s.Requests != 2 || s.TotalTokens != 9 || s.DurationMS != 3000 || s.EstimatedRecords != 1 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}
func TestToday(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	xs := []Record{{Time: now.Add(-2 * time.Hour)}, {Time: now.Add(-24 * time.Hour)}}
	if got := len(Today(xs, now)); got != 1 {
		t.Fatalf("got %d", got)
	}
}
