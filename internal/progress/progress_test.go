package progress

import (
	"testing"

	"github.com/n25tech/bookman/internal/db"
)

func TestComputeLastWriteWins(t *testing.T) {
	stored := &db.Progress{BookID: 1, Page: 10, ClientTS: 1000, Device: "a"}

	if got, ok := Compute(stored, Input{Page: 3, ClientTS: 999, Device: "b"}, 1, 100, 5); ok || got.Page != 10 {
		t.Fatalf("older save must lose, got %+v accepted=%v", got, ok)
	}
	if _, ok := Compute(stored, Input{Page: 3, ClientTS: 1000, Device: "b"}, 1, 100, 5); ok {
		t.Fatal("equal client_ts must lose")
	}
	got, ok := Compute(stored, Input{Page: 3, Offset: 0.5, ClientTS: 1001, Device: "b"}, 1, 100, 5)
	if !ok || got.Page != 3 || got.Device != "b" || got.UpdatedAt != 5 {
		t.Fatalf("newer save must win, got %+v", got)
	}
	if want := 2.5 / 100; got.Percent != want {
		t.Fatalf("percent = %v, want %v", got.Percent, want)
	}
	if got, ok := Compute(nil, Input{Page: 1, ClientTS: 1}, 1, 100, 5); !ok || got.Page != 1 {
		t.Fatal("first save must win")
	}
}

func TestComputeClampsAndFinishes(t *testing.T) {
	got, _ := Compute(nil, Input{Page: 500, Offset: 7, ClientTS: 1}, 1, 20, 0)
	if got.Page != 20 || got.Offset != 1 || !got.Finished || got.Percent > 1 {
		t.Fatalf("clamp/finish failed: %+v", got)
	}
	got, _ = Compute(nil, Input{Page: 20, Offset: 0.89, ClientTS: 1}, 1, 20, 0)
	if got.Finished {
		t.Fatal("offset < 0.9 on last page must not finish")
	}
	got, _ = Compute(nil, Input{Page: -3, Offset: -1, ClientTS: 1}, 1, 20, 0)
	if got.Page != 1 || got.Offset != 0 {
		t.Fatalf("lower clamp failed: %+v", got)
	}
	// A finished book stays finished when re-read, unless the client says otherwise.
	stored := &db.Progress{Page: 20, Finished: true, ClientTS: 1}
	got, _ = Compute(stored, Input{Page: 2, ClientTS: 2}, 1, 20, 0)
	if !got.Finished {
		t.Fatal("finished flag should be kept")
	}
	f := false
	got, _ = Compute(stored, Input{Page: 2, ClientTS: 3, Finished: &f}, 1, 20, 0)
	if got.Finished {
		t.Fatal("explicit finished=false should win")
	}
}

func TestNextSession(t *testing.T) {
	last := &db.ReadingSession{EndedAt: 1_000_000}
	cases := []struct {
		name string
		last *db.ReadingSession
		ts   int64
		want SessionAction
	}{
		{"first save", nil, 5, SessionNew},
		{"soon after", last, 1_000_000 + 60_000, SessionExtend},
		{"exactly the gap", last, 1_000_000 + SessionGap, SessionExtend},
		{"after the gap", last, 1_000_000 + SessionGap + 1, SessionNew},
		{"out of order", last, 999_999, SessionNone},
	}
	for _, c := range cases {
		if got := NextSession(c.last, c.ts); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
