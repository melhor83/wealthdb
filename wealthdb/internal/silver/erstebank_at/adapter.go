// Package erstebankat projects the erstebank_at collector's silver (an
// Austrian Erste Bank und Sparkassen / George retail relationship: checking
// accounts and credit cards) into canonical change records. See
// collectors/erstebank_at/ for the bronze/silver schema; this package is the
// gold mapping.
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
//     amount owed; gold carries a card as negative cash, so signedBalance
//     negates every card figure and passes a checking figure through.
//
//   - TRANSACTIONS: both ledgers, keyed by George's row id. Amounts are
//     already signed the canonical way (money out negative). The kind comes
//     from George's transaction type (transactions.go); information rows that
//     book no amount are left out.
package erstebankat

import (
	"context"
	"database/sql"

	"github.com/smartpointer-com/wealthdb/wealthdb/internal/silver"
)

// kindName is the silver_kind / collector name (the whitelist entry the
// 0173 gold migration adds).
const kindName = "erstebank_at"

func init() {
	silver.Register(&Adapter{})
}

type Adapter struct{}

func (*Adapter) Kind() string { return kindName }

func (*Adapter) Open(_ context.Context, spec silver.OpenSpec) (silver.Connection, error) {
	db, err := silver.OpenReadOnlySQLite(spec.Path, "erstebank_at silver")
	if err != nil {
		return nil, err
	}
	return &Connection{db: db}, nil
}

type Connection struct {
	db *sql.DB
}

func (c *Connection) Close() error { return silver.CloseDB(&c.db) }
