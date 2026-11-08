package erstebankat

import "github.com/smartpointer-com/wealthdb/wealthdb/internal/returns"

// init registers this source's co-located ReturnsPolicy, beside the
// silver.Register(&Adapter{}) call. The checking accounts are cash conduits;
// see returns.DepositBankPolicy for the flow-complete + hidden-plumbing
// rationale. The credit cards need nothing of their own: the engine drops
// `card` accounts at the loader (gold.returnsInvisibleKind).
func init() {
	returns.RegisterPolicy(kindName, returns.DepositBankPolicy())
}
