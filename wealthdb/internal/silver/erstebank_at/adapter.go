// Package erstebankat projects the erstebank_at collector's silver (an
// Austrian Erste Bank und Sparkassen / George retail relationship: checking
// accounts and credit cards) into canonical change records. See
// collectors/erstebank_at/ for the bronze/silver schema; this package is the
// gold mapping. The projection is the shared bank adapter's (silver/bank);
// this file declares the source.
//
//   - One ACCOUNT per silver account: AccountKind 'cash' for a checking
//     account, 'card' for a credit card. DisplayName is George's product name
//     with the last-4 mask; TaxWrapper 'taxable_personal', ManagementStyle
//     'self_directed'. All overridable via account_overrides.
//
//   - CASH BALANCES, never positions. A CURRENT balance from the roster at
//     each run that listed the account (silver's run_accounts), and a CLOSING
//     balance for every day the derived daily-balance series carries, stamped
//     at the day's last second. Silver holds a card's balance as the POSITIVE
//     amount owed; gold carries a card as negative cash, so the bank adapter
//     negates every card figure and passes a checking figure through.
//
//   - TRANSACTIONS: both ledgers, keyed by George's row id. Amounts are
//     already signed the canonical way (money out negative). The kind comes
//     from George's transaction type (kinds.go); information rows that book
//     no amount are left out.
//
//   - TEXT COLUMNS, verbatim from silver: the description is the row's
//     title, the memo its free-text lines (the payer's purpose text, kept
//     out of the merchant signature), the counterparty the payment partner
//     or card merchant, and the provider category George's own sub-category
//     (which the holder can re-file, so it never sets a kind).
package erstebankat

import (
	"fmt"
	"strings"

	"github.com/smartpointer-com/wealthdb/wealthdb/internal/canonical"
	"github.com/smartpointer-com/wealthdb/wealthdb/internal/returns"
	"github.com/smartpointer-com/wealthdb/wealthdb/internal/silver"
	"github.com/smartpointer-com/wealthdb/wealthdb/internal/silver/bank"
)

// kindName is the silver_kind / collector name.
const kindName = "erstebank_at"

// silverCard is silver's account_kind for a credit card; every other account
// is a checking account.
const silverCard = "credit_card"

// spanExtrema is the MIN/MAX of every date the projection touches: account
// snapshots, the loaded runs (where the CURRENT balances are stamped), the
// ledger (posted_at), and the daily closes at each day's last second.
var spanExtrema = fmt.Sprintf(`
SELECT MIN(t), MAX(t) FROM (
    SELECT snapshot_at AS t FROM accounts
    UNION ALL SELECT snapshot_at FROM dump_runs
    UNION ALL SELECT posted_at FROM transactions
    UNION ALL SELECT balance_date + %d FROM daily_balances)`, silver.DayEndOffset)

// adapter projects the source.
var adapter = bank.New(bank.Source{
	Kind:            kindName,
	DefaultCurrency: "EUR",
	// The bank policy: the ledger is the complete cash history. The engine
	// hides a cash account's own rows on any source (gold.hiddenConstituent)
	// and drops a card at load (gold.returnsInvisibleKind).
	Returns: returns.DefaultReturnsPolicy(returns.BankFlowPolicy()),
	// The shared load clock (silver.LoadClock): this source has no change
	// feed of its own, so a loaded dump is the trigger and a full re-emit
	// is the response.
	Clock:       silver.LoadClock(kindName, spanExtrema, silver.Range("transactions", "posted_at")),
	NameCol:     "display_name",
	ProductCol:  "account_kind",
	CardProduct: silverCard,
	Display:     func(r bank.RosterRow) string { return strings.TrimSpace(r.Name + " " + r.Mask) },
	Listing:     "run_accounts",
	Closings: []bank.Series{{
		Label: "daily balances",
		Basis: "daily_balance",
		Query: `
SELECT account_external_id, balance_date, balance
  FROM daily_balances
 WHERE balance_date BETWEEN ? AND ?
 ORDER BY account_external_id, balance_date`,
		DayEnd: true,
	}},
	IDCol: "txn_id",
	// Information rows (balance_relevant = 0) book no amount and move no
	// balance.
	Where: "balance_relevant = 1",
	Cols: bank.LedgerCols{
		Kind: "tx_type", Description: "description", Counterparty: "counterparty",
		Memo: "reference", Currency: "currency",
		Category: "json_extract(payload, '$.categories[0].subCategory')",
	},
	// SIGNS. Silver holds every amount as George states it: money out
	// negative, money in positive. A checking row keeps that sign; a card
	// row is routed through the canonical sign, because the card kinds have
	// a fixed direction.
	TxKind: func(r bank.Row, card bool) (canonical.TxKind, bool, bool) {
		if card {
			k, known := cardTxKind(r.Kind, hasMerchantCode(r.Payload), r.Amount)
			return k, known, true
		}
		k, known := checkingTxKind(r.Kind, r.Description+" "+r.Memo, r.Amount)
		return k, known, false
	},
})

// init registers the kind (silver.Source).
func init() { silver.Register(adapter.Source()) }
