package systemacct

import "testing"

func TestStableIDs(t *testing.T) {
	if Cash("USD") != Cash("USD") {
		t.Fatal("cash id must be stable")
	}
	if Cash("USD") == LoanFunding("USD") {
		t.Fatal("cash and funding accounts must differ")
	}
	if !AllowsNegative(Cash("USD")) || !AllowsNegative(LoanFunding("USD")) {
		t.Fatal("system accounts must allow a negative balance")
	}
	if AllowsNegative("11111111-1111-1111-1111-111111111111") {
		t.Fatal("customer accounts must not allow a negative balance")
	}
}
