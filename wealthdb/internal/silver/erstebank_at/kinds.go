package erstebankat

import (
	"encoding/json"
	"strings"

	"github.com/smartpointer-com/wealthdb/wealthdb/internal/canonical"
)

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

// hasMerchantCode reports whether a card row's payload carries a merchant
// category code, which marks a credit as a merchant refund.
func hasMerchantCode(payload string) bool {
	var p struct {
		MCC any `json:"mcc"`
	}
	if json.Unmarshal([]byte(payload), &p) != nil {
		return false
	}
	switch v := p.MCC.(type) {
	case nil:
		return false
	case string:
		return v != ""
	default:
		return true
	}
}
