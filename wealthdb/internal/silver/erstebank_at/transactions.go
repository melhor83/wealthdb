package erstebankat

import (
	"context"
	"fmt"
	"strings"

	"github.com/smartpointer-com/wealthdb/wealthdb/internal/canonical"
	"github.com/smartpointer-com/wealthdb/wealthdb/internal/silver"
)

// Transactions yields both ledgers, checking and card, keyed by George's row
// id.
//
// SIGNS. Silver holds every amount as George states it: money out negative,
// money in positive. On a checking account that is the canonical sign, so it
// is preserved. On a card it agrees with the canonical convention for a
// liability carried as negative cash; it is still routed through
// ApplyCanonicalSign, because the card kinds have a fixed canonical
// direction.
//
// Information rows (balance_relevant = 0) book no amount and move no
// balance, so they are left out.
//
// TEXT COLUMNS, verbatim from silver:
//
//   - description: the row's title, George's booking text;
//   - memo: the free-text lines (the payer's purpose text), which gold
//     keeps out of the merchant signature;
//   - counterparty: the payment partner, else the card merchant;
//   - provider_category: George's own sub-category (`categories[0]`),
//     which George lets the account holder re-file, so it never sets a kind.
func (c *Connection) Transactions(ctx context.Context, w canonical.Window) (silver.TransactionStream, error) {
	if !w.HasChanges {
		return silver.NewTransactionStream(canonical.TransactionBatch{}), nil
	}
	ccy, err := silver.ReadAccountCurrencies(ctx, c.db, "EUR")
	if err != nil {
		return nil, err
	}
	cards, err := c.cardAccounts(ctx)
	if err != nil {
		return nil, err
	}
	const q = `
SELECT txn_id, posted_at, account_external_id, amount, COALESCE(currency, ''),
       COALESCE(tx_type, ''), COALESCE(description, ''), COALESCE(counterparty, ''),
       COALESCE(reference, ''),
       COALESCE(json_extract(payload, '$.categories[0].subCategory'), ''),
       COALESCE(CAST(json_extract(payload, '$.mcc') AS TEXT), '') <> '',
       payload
  FROM transactions
 WHERE balance_relevant = 1 AND posted_at BETWEEN ? AND ?
 ORDER BY posted_at, txn_id`
	rows, err := c.db.QueryContext(ctx, q, w.Start, w.End)
	if err != nil {
		return nil, fmt.Errorf("erstebank_at Transactions: %w", err)
	}
	defer rows.Close()

	var out canonical.TransactionBatch
	for rows.Next() {
		var (
			txnID, id, rowCcy, txType, desc string
			cp, ref, category, payload      string
			posted                          int64
			amtFloat                        float64
			merchant                        bool
		)
		if err := rows.Scan(&txnID, &posted, &id, &amtFloat, &rowCcy, &txType, &desc, &cp,
			&ref, &category, &merchant, &payload); err != nil {
			return nil, err
		}
		// The collector rounds money to cents before storing, so the
		// float→decimal step is exact at display precision.
		amt := canonical.NewDecimalFromFloat(amtFloat)
		net := &amt
		var (
			kind  canonical.TxKind
			known bool
		)
		if cards[id] {
			kind, known = cardTxKind(txType, merchant, amt)
			net = canonical.ApplyCanonicalSign(kind, &amt)
		} else {
			kind, known = checkingTxKind(txType, desc+" "+ref, amt)
		}
		extra := map[string]any{}
		if !known {
			extra["source_kind"] = txType
		}
		currency := rowCcy
		if currency == "" {
			currency = ccy.Of(id)
		}
		out.Transactions = append(out.Transactions, canonical.TransactionChange{
			TransactionExternalID: txnID,
			OccurredAt:            posted,
			AccountExternalID:     id,
			Kind:                  kind,
			Currency:              currency,
			GrossAmount:           net,
			NetAmount:             net,
			Description:           silver.StrPtrIfNonEmpty(desc),
			Memo:                  silver.StrPtrIfNonEmpty(ref),
			// The partner or merchant verbatim: it is the input to gold's
			// merchant signature, so it is never reformatted.
			Counterparty:     silver.StrPtrIfNonEmpty(cp),
			ProviderCategory: silver.StrPtrIfNonEmpty(category),
			Payload:          silver.PayloadWith(payload, extra),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return silver.NewTransactionStream(out), nil
}

// cardAccounts is the set of accounts silver records as credit cards.
func (c *Connection) cardAccounts(ctx context.Context) (map[string]bool, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT DISTINCT account_external_id FROM accounts WHERE account_kind = ?`, silverCard)
	if err != nil {
		return nil, fmt.Errorf("erstebank_at card accounts: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// checkingTypes are the George transaction types (`txType`) of a checking
// row whose direction is its sign: a card payment, an ATM withdrawal, a
// direct debit (CASHCOLLECTION), a standing order, a transfer entered by
// hand or by the bank (MANUAL, OTHER).
var checkingTypes = map[string]bool{
	"ATM": true, "CASHCOLLECTION": true, "MANUAL": true, "OTHER": true,
	"POSPAYMENT": true, "STANDINGORDER": true,
}

// checkingTxKind maps a checking row and reports whether its type was
// recognised. An unrecognised type lands as `other`, its raw value kept in
// the payload (docs/DESIGN.md §6.8).
//
// BANKFEE is the account-closing entry (Abschluss), which carries the
// account fee, the interest either way, and the capital-gains tax withheld
// on credit interest. Its text tells them apart: KESt (Kapitalertragsteuer)
// marks the tax, Zins the interest, and anything else is a fee.
func checkingTxKind(txType, text string, amt canonical.Decimal) (canonical.TxKind, bool) {
	if txType == "BANKFEE" {
		t := strings.ToUpper(text)
		switch {
		case strings.Contains(t, "KEST"):
			return canonical.TxKindTax, true
		case strings.Contains(t, "ZINS"):
			return canonical.TxKindInterest, true
		default:
			return canonical.TxKindFee, true
		}
	}
	if !checkingTypes[txType] {
		return canonical.TxKindOther, false
	}
	if amt.IsNegative() {
		return canonical.TxKindWithdrawal, true
	}
	return canonical.TxKindDeposit, true
}

// cardTxKind maps a card row and reports whether its type was recognised.
// George books every card row as CREDITCARD, so the kind comes from the
// row's shape: a charge is a purchase, a credit from a merchant (it carries
// the merchant category code) a refund, and a credit with no merchant the
// repayment of the card.
func cardTxKind(txType string, merchant bool, amt canonical.Decimal) (canonical.TxKind, bool) {
	switch {
	case txType != "CREDITCARD":
		return canonical.TxKindOther, false
	case amt.IsNegative():
		return canonical.TxKindPurchase, true
	case merchant:
		return canonical.TxKindRefund, true
	default:
		return canonical.TxKindCardPayment, true
	}
}
