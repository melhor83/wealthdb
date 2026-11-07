package gold

import (
	"math"
	"testing"
	"time"

	"github.com/smartpointer-com/wealthdb/wealthdb/internal/canonical"
	"github.com/smartpointer-com/wealthdb/wealthdb/internal/returns"
)

// TestMWRReadsTheChainsValues: a month-end account beside a daily one takes a
// deposit that its next valuation shows. Where a window ends, or starts, between
// the deposit and that valuation, the chain reads the account with the deposit
// rolled in. MWR reads the same values, so the two agree. start, end and
// net_flow stay the carried values.
func TestMWRReadsTheChainsValues(t *testing.T) {
	t.Parallel()
	jan31, feb28 := dy(2023, time.January, 31), dy(2023, time.February, 28)
	feb := func(d int) int64 { return dy(2023, time.February, d) }
	dep := canonical.TxKindDeposit
	cases := []struct {
		name     string
		from, to int64
		daily    []snap
		dailyTx  []txn
		start    float64
		rolled   bool
	}{
		{"a deposit after the last valuation, inside the window", 0, feb(20),
			dailyFrom(from(10000)), nil, 11000, true},
		{"nothing unshown", 0, feb28,
			dailyFrom(from(10000, [2]int64{20, 10500})), []txn{{feb(20), dep, 500}}, 11000, false},
		{"a deposit before the window's start, shown inside it", feb(15), feb28,
			dailyFrom(from(10000, [2]int64{20, 10500})), []txn{{feb(20), dep, 500}}, 11000, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			db, ctx := openMigrated(t)
			seedReturnsSource(t, db, ctx, "bankx", "fidelity")
			seedAcct(t, db, ctx, "bankx", "DAILY", canonical.AccountKindBrokerage, nil, c.daily, c.dailyTx)
			seedAcct(t, db, ctx, "bankx", "MONTHLY", canonical.AccountKindBrokerage, nil,
				[]snap{{jan31, 1000}, {feb28, 2000}}, []txn{{feb(10), dep, 1000}})
			rows, err := RunReturns(ctx, db, params("sources", c.from, c.to+86399))
			if err != nil {
				t.Fatalf("RunReturns: %v", err)
			}
			s, ok := summaryFor(rows, "bankx")
			if !ok || s.TWR == nil || s.MWR == nil {
				t.Fatalf("no TWR or MWR: %+v", s)
			}
			if math.Abs(*s.TWR) > 1e-9 || math.Abs(*s.MWR) > 1e-6 {
				t.Errorf("TWR %.6f, MWR %.6f, want both 0", *s.TWR, *s.MWR)
			}
			if v, _ := parseFloatPtr(s.StartValue); v != c.start {
				t.Errorf("start = %.2f, want the carried %.2f", v, c.start)
			}
			if got := qualityHas(s, "rolled_forward"); got != c.rolled {
				t.Errorf("rolled_forward = %v, want %v: %v", got, c.rolled, s.Quality)
			}
		})
	}
}

// TestMWRUnchangedWithNothingUnshown: with every move shown at both edges, MWR
// is the XIRR of the carried values and the flows, as it always was.
func TestMWRUnchangedWithNothingUnshown(t *testing.T) {
	t.Parallel()
	db, ctx := openMigrated(t)
	seedReturnsSource(t, db, ctx, "bankx", "fidelity")
	jan31, feb28 := dy(2023, time.January, 31), dy(2023, time.February, 28)
	feb10 := dy(2023, time.February, 10)
	seedAcct(t, db, ctx, "bankx", "A", canonical.AccountKindBrokerage, nil,
		[]snap{{jan31, 10000}, {feb10, 11000}, {feb28, 11500}}, []txn{{feb10, canonical.TxKindDeposit, 1000}})
	rows, err := RunReturns(ctx, db, params("sources", 0, eod(2023, time.February, 28)))
	if err != nil {
		t.Fatalf("RunReturns: %v", err)
	}
	s, _ := summaryFor(rows, "bankx")
	if s.MWR == nil {
		t.Fatalf("no MWR: %+v", s)
	}
	from, to := EpochDay(jan31), EpochDay(feb28)
	rate, err := returns.XIRR(10000, 11500, from, to, []returns.Flow{{Day: EpochDay(feb10), Amount: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if want := returns.DeAnnualize(rate, float64(to-from)); math.Abs(*s.MWR-want) > 1e-12 {
		t.Errorf("MWR %.9f, want %.9f", *s.MWR, want)
	}
	if qualityHas(s, "rolled_forward") {
		t.Errorf("rolled_forward with nothing unshown: %v", s.Quality)
	}
}
