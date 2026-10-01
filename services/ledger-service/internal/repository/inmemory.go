package repository

import (
	"context"
	"sync"

	"github.com/shopspring/decimal"

	"banking-platform/pkg/apierror"
	"banking-platform/pkg/systemacct"
	"banking-platform/services/ledger-service/internal/domain"
)

// InMemory is a concurrency-safe in-memory ledger (used when Postgres is
// disabled). It preserves the same invariants as the Postgres implementation.
type InMemory struct {
	mu          sync.Mutex
	accounts    map[string]*domain.Account
	appliedKeys map[string]struct{} // idempotency keys already applied
}

// NewInMemory builds an empty in-memory ledger.
func NewInMemory() *InMemory {
	return &InMemory{
		accounts:    map[string]*domain.Account{},
		appliedKeys: map[string]struct{}{},
	}
}

func (r *InMemory) OpenAccount(_ context.Context, accountID, currency string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[accountID]; ok {
		return nil // idempotent
	}
	r.accounts[accountID] = &domain.Account{AccountID: accountID, Currency: currency, Balance: decimal.Zero}
	return nil
}

func (r *InMemory) Post(_ context.Context, t domain.Transaction) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.appliedKeys[t.IdempotencyKey]; ok {
		return true, nil
	}

	// Validate all accounts exist, then apply. Customer accounts cannot go negative.
	// The mutex serializes check and apply so concurrent transfers cannot overdraw.
	next := map[string]decimal.Decimal{}
	for _, e := range t.Entries {
		acc, ok := r.accounts[e.AccountID]
		if !ok {
			return false, apierror.ErrBadRequest("ledger account not found: " + e.AccountID)
		}
		if acc.Currency != t.Currency {
			return false, apierror.ErrBadRequest("currency mismatch for account " + e.AccountID)
		}
		bal := acc.Balance
		if prev, ok := next[e.AccountID]; ok {
			bal = prev
		}
		if e.Direction == domain.Credit {
			bal = bal.Add(e.Amount)
		} else {
			bal = bal.Sub(e.Amount)
		}
		next[e.AccountID] = bal
	}
	for id, bal := range next {
		if bal.IsNegative() && !systemacct.AllowsNegative(id) {
			return false, apierror.New(422, "insufficient_funds", "insufficient funds")
		}
	}
	for id, bal := range next {
		r.accounts[id].Balance = bal
	}
	r.appliedKeys[t.IdempotencyKey] = struct{}{}
	return false, nil
}

func (r *InMemory) Balance(_ context.Context, accountID string) (decimal.Decimal, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.accounts[accountID]
	if !ok {
		return decimal.Zero, "", apierror.ErrNotFound("ledger account not found")
	}
	return acc.Balance, acc.Currency, nil
}
