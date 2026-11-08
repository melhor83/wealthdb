package erstebankat

import (
	"context"
	"fmt"
	"strings"

	"github.com/smartpointer-com/wealthdb/wealthdb/internal/canonical"
	"github.com/smartpointer-com/wealthdb/wealthdb/internal/silver"
)

// silverCard is silver's account_kind for a credit card; every other account
// is a checking account.
const silverCard = "credit_card"

// Snapshots emits, in one batch: one ACCOUNT per checking account or card, a
// CURRENT balance per run whose roster listed the account (silver's
// run_accounts), and a CLOSING balance per day of the daily-balance series.
// Cash is modelled as cash_balances, never a position or instrument.
func (c *Connection) Snapshots(ctx context.Context, w canonical.Window) (silver.SnapshotStream, error) {
	if !w.HasChanges {
		return silver.NewSnapshotStream(nil), nil
	}
	ccy, err := silver.ReadAccountCurrencies(ctx, c.db, "EUR")
	if err != nil {
		return nil, err
	}
	roster, err := silver.ReadBankRoster(ctx, c.db, "erstebank_at appendAccounts", "display_name", "account_kind")
	if err != nil {
		return nil, err
	}
	cards := map[string]bool{}
	var batch canonical.SnapshotBatch
	for _, r := range roster {
		kind := canonical.AccountKindCash
		if r.AccountType == silverCard {
			kind = canonical.AccountKindCard
			cards[r.ID] = true
		}
		batch.Accounts = append(batch.Accounts, r.Account(kind, displayName(r.Name, r.Mask), ccy.Of(r.ID)))
	}
	sign := func(id string, v float64) canonical.Decimal { return signedBalance(v, cards[id]) }
	if err := silver.AppendReportedBalances(ctx, c.db, kindName, "run_accounts", w, ccy.Of, sign, &batch); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`
SELECT account_external_id, balance_date + %[1]d, balance
  FROM daily_balances
 WHERE balance_date + %[1]d BETWEEN ? AND ?
 ORDER BY account_external_id, balance_date`, silver.DayEndOffset)
	if err := silver.AppendClosingBalances(ctx, c.db, "erstebank_at daily balances",
		q, []any{w.Start, w.End}, `{"basis":"daily_balance"}`, ccy.Of, sign, &batch); err != nil {
		return nil, err
	}
	return silver.NewSnapshotStream([]canonical.SnapshotBatch{batch}), nil
}

// displayName is George's product name with the last-4 mask (e.g.
// "Girokonto …1234"), or either alone.
func displayName(product, mask string) string {
	return strings.TrimSpace(product + " " + mask)
}

// signedBalance converts a silver balance to the canonical sign. Silver
// holds a card's balance the fleet's way, as the POSITIVE amount owed, and
// gold carries a revolving-credit liability as negative cash, so a card
// figure is negated and a checking figure passes through. This is the only
// place the flip happens.
//
// The collector rounds money to cents before storing, so the REAL→Decimal
// step is exact at display precision.
func signedBalance(amount float64, card bool) canonical.Decimal {
	d := canonical.NewDecimalFromFloat(amount)
	if card {
		return d.Neg()
	}
	return d
}
