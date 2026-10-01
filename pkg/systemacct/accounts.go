// Package systemacct holds deterministic ledger account IDs for platform
// contra accounts. These accounts are allowed to go negative: they are the
// other side of customer deposits, card spend, and loan funding.
package systemacct

import "github.com/google/uuid"

// Namespaces are stable so every service derives the same account IDs.
var (
	cashNamespace    = uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	fundingNamespace = uuid.MustParse("00000000-0000-0000-0000-0000000000dd")
)

// Cash is the system cash account for a currency (deposit contra, card settlement).
func Cash(currency string) string {
	return uuid.NewSHA1(cashNamespace, []byte("system-cash-"+currency)).String()
}

// LoanFunding is the system funding account for loan disbursement and repayment.
func LoanFunding(currency string) string {
	return uuid.NewSHA1(fundingNamespace, []byte("system-loan-funding-"+currency)).String()
}

// AllowsNegative reports whether a ledger account is a platform contra account.
func AllowsNegative(accountID string) bool {
	return accountID == Cash("USD") || accountID == LoanFunding("USD")
}
