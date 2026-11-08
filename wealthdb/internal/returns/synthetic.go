package returns

import "math"

// syntheticTol is the absolute (output-currency) value below which no
// synthetic flow is booked; OnboardingFlow and ClosureFlow also suppress a
// negative amount (a per-entity-once step-up fully netted by sibling funding
// drops, a closure with nothing left to book).
const syntheticTol = 1e-6

// OnboardingFlow returns the synthetic onboarding inflow for an aggregate
// constituent's debut, so the step-up in the aggregate value series on the day
// a constituent first appears is booked as capital-in rather than performance.
// The caller subsumes the constituent's real pre-debut flows first
// (entityFlows), so the synthetic books the whole amount it is given; ~0/negative
// amounts are suppressed. The returned Flow is dated on the debut day with
// capital-in (positive) sign.
func OnboardingFlow(debutDay int64, firstValue float64) (Flow, bool) {
	if firstValue <= syntheticTol {
		return Flow{}, false
	}
	return Flow{Day: debutDay, Amount: firstValue, Synthetic: true}, true
}

// FundedOnboardingFlow is OnboardingFlow for a debut whose pre-debut legs netting
// cancelled against money the entity already held, funded being their signed sum.
// It books what the first value holds beyond those legs. That is an outflow where
// the account passed on or paid out more than it kept, so this amount is not
// suppressed when negative: what happens before a first valuation is capital in
// either direction, never performance.
func FundedOnboardingFlow(debutDay int64, firstValue, funded float64) (Flow, bool) {
	if funded == 0 {
		return OnboardingFlow(debutDay, firstValue)
	}
	amount := firstValue - funded
	if math.Abs(amount) <= syntheticTol {
		return Flow{}, false
	}
	return Flow{Day: debutDay, Amount: amount, Synthetic: true}, true
}

// ClosureFlow returns the closure outflow for an explicitly-closed aggregate
// constituent, dated on zeroingDay: the day its value first shows it empty.
//
// The caller passes the amount that leaves: on a final-day zeroing the value
// carried the day before, less what already left; inside a longer zero tail
// what the tail's exit legs moved out. Under ClosureSubsumeDrains the caller
// subsumes the real closure-window flows, so no near-day dedup is needed;
// under ClosureLedgerExact the caller keeps the real flows and skips this
// entirely. ok is false when there is no explicit closure (zeroingDay==0) or
// nothing to book. Staleness/dormancy must never reach here.
func ClosureFlow(zeroingDay int64, amount float64) (Flow, bool) {
	if zeroingDay == 0 {
		return Flow{}, false
	}
	if amount <= syntheticTol {
		return Flow{}, false // nothing left to book
	}
	return Flow{Day: zeroingDay, Amount: -amount, Synthetic: true}, true
}

// ZeroedValue zeroes a constituent from its closure day, the last day of its
// zero tail: its contribution to an aggregate boundary value is its
// carried-forward value before that day and exactly 0 from it onward. On a
// final-day closure that is the day the closure flow books its carried value.
// closureDay==0 means "not closed" (value passes through unchanged).
func ZeroedValue(value float64, day, closureDay int64) float64 {
	if closureDay != 0 && day >= closureDay {
		return 0
	}
	return value
}
