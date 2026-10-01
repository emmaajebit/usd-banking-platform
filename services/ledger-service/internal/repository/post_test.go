package repository

import (
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"banking-platform/pkg/systemacct"
	"banking-platform/services/ledger-service/internal/domain"
)

func TestPostRejectsOverdraftAndAllowsSystemNegative(t *testing.T) {
	r := NewInMemory()
	customer := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	cash := systemacct.Cash("USD")
	if err := r.OpenAccount(t.Context(), customer, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := r.OpenAccount(t.Context(), cash, "USD"); err != nil {
		t.Fatal(err)
	}

	_, err := r.Post(t.Context(), domain.Transaction{
		ID: "t1", IdempotencyKey: "k1", Currency: "USD",
		Entries: []domain.Entry{
			{AccountID: customer, Direction: domain.Debit, Amount: decimal.RequireFromString("10")},
			{AccountID: cash, Direction: domain.Credit, Amount: decimal.RequireFromString("10")},
		},
	})
	if err == nil {
		t.Fatal("expected insufficient funds")
	}

	if _, err := r.Post(t.Context(), domain.Transaction{
		ID: "t2", IdempotencyKey: "k2", Currency: "USD",
		Entries: []domain.Entry{
			{AccountID: cash, Direction: domain.Debit, Amount: decimal.RequireFromString("25")},
			{AccountID: customer, Direction: domain.Credit, Amount: decimal.RequireFromString("25")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	bal, _, err := r.Balance(t.Context(), customer)
	if err != nil || !bal.Equal(decimal.RequireFromString("25")) {
		t.Fatalf("customer balance = %s err=%v", bal, err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = r.Post(t.Context(), domain.Transaction{
				ID: "race", IdempotencyKey: "race-" + string(rune('a'+i)), Currency: "USD",
				Entries: []domain.Entry{
					{AccountID: customer, Direction: domain.Debit, Amount: decimal.RequireFromString("10")},
					{AccountID: cash, Direction: domain.Credit, Amount: decimal.RequireFromString("10")},
				},
			})
		}(i)
	}
	wg.Wait()
	bal, _, _ = r.Balance(t.Context(), customer)
	if bal.IsNegative() {
		t.Fatalf("customer balance went negative: %s", bal)
	}
}
