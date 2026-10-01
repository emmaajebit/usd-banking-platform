// Package service implements transaction-service use cases: deposits,
// withdrawals and transfers. Every money movement is a balanced ledger posting
// plus a recorded transaction and (transactionally) an outbox event.
package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"banking-platform/pkg/apierror"
	"banking-platform/pkg/outbox"
	"banking-platform/pkg/systemacct"
	"banking-platform/services/transaction-service/internal/domain"
	"banking-platform/services/transaction-service/internal/ledgerclient"
	"banking-platform/services/transaction-service/internal/repository"
)

// EventTopic is the Kafka topic transaction events are published to.
const EventTopic = "banking.transactions"

// systemAccount is the platform cash contra account for a currency.
func systemAccount(currency string) string { return systemacct.Cash(currency) }

// Ledger is the subset of the ledger transaction-service depends on.
type Ledger interface {
	OpenAccount(ctx context.Context, accountID, currency string) error
	Post(ctx context.Context, txID, idemKey, currency, reference string, entries []ledgerclient.Entry) (string, bool, error)
	Balance(ctx context.Context, accountID string) (balance, currency string, err error)
}

// Service holds transaction-service dependencies.
type Service struct {
	repo       repository.Repository
	ledger     Ledger
	source     string
	emitEvents bool
	log        *zap.Logger
}

// New builds the Service. emitEvents enables the transactional outbox (requires
// Postgres).
func New(repo repository.Repository, ledger Ledger, source string, emitEvents bool, log *zap.Logger) *Service {
	return &Service{repo: repo, ledger: ledger, source: source, emitEvents: emitEvents, log: log}
}

// Deposit credits an account (DEBIT system cash, CREDIT account).
func (s *Service) Deposit(ctx context.Context, accountID, amountStr, currency, idemKey, ref string) (domain.Transaction, error) {
	amount, err := parsePositive(amountStr)
	if err != nil {
		return domain.Transaction{}, err
	}
	if currency != "USD" {
		return domain.Transaction{}, apierror.ErrBadRequest("only USD is supported")
	}
	if replay, ok := s.replay(ctx, idemKey); ok {
		return replay, nil
	}
	sys := systemAccount(currency)
	if err := s.ledger.OpenAccount(ctx, sys, currency); err != nil {
		return domain.Transaction{}, ledgerErr(err)
	}
	entries := []ledgerclient.Entry{
		{AccountID: sys, Direction: "DEBIT", Amount: amount.String()},
		{AccountID: accountID, Direction: "CREDIT", Amount: amount.String()},
	}
	return s.execute(ctx, domain.Transaction{
		Type: domain.TypeDeposit, ToAccountID: accountID, Amount: amount, Currency: currency,
		IdempotencyKey: idemKey, Reference: ref,
	}, entries)
}

// Withdraw debits an account (DEBIT account, CREDIT system cash) after an
// overdraft check.
func (s *Service) Withdraw(ctx context.Context, accountID, amountStr, currency, idemKey, ref string) (domain.Transaction, error) {
	amount, err := parsePositive(amountStr)
	if err != nil {
		return domain.Transaction{}, err
	}
	if currency != "USD" {
		return domain.Transaction{}, apierror.ErrBadRequest("only USD is supported")
	}
	if replay, ok := s.replay(ctx, idemKey); ok {
		return replay, nil
	}
	sys := systemAccount(currency)
	if err := s.ledger.OpenAccount(ctx, sys, currency); err != nil {
		return domain.Transaction{}, ledgerErr(err)
	}
	entries := []ledgerclient.Entry{
		{AccountID: accountID, Direction: "DEBIT", Amount: amount.String()},
		{AccountID: sys, Direction: "CREDIT", Amount: amount.String()},
	}
	return s.execute(ctx, domain.Transaction{
		Type: domain.TypeWithdrawal, FromAccountID: accountID, Amount: amount, Currency: currency,
		IdempotencyKey: idemKey, Reference: ref,
	}, entries)
}

// Transfer moves money between two accounts (DEBIT from, CREDIT to).
func (s *Service) Transfer(ctx context.Context, from, to, amountStr, currency, idemKey, ref string) (domain.Transaction, error) {
	amount, err := parsePositive(amountStr)
	if err != nil {
		return domain.Transaction{}, err
	}
	if currency != "USD" {
		return domain.Transaction{}, apierror.ErrBadRequest("only USD is supported")
	}
	if from == to {
		return domain.Transaction{}, apierror.ErrBadRequest("cannot transfer to the same account")
	}
	if replay, ok := s.replay(ctx, idemKey); ok {
		return replay, nil
	}
	entries := []ledgerclient.Entry{
		{AccountID: from, Direction: "DEBIT", Amount: amount.String()},
		{AccountID: to, Direction: "CREDIT", Amount: amount.String()},
	}
	return s.execute(ctx, domain.Transaction{
		Type: domain.TypeTransfer, FromAccountID: from, ToAccountID: to, Amount: amount, Currency: currency,
		IdempotencyKey: idemKey, Reference: ref,
	}, entries)
}

// Get returns a transaction by ID.
func (s *Service) Get(ctx context.Context, id string) (domain.Transaction, error) {
	return s.repo.GetByID(ctx, id)
}

// ListByAccount returns an account's transaction history.
func (s *Service) ListByAccount(ctx context.Context, accountID string) ([]domain.Transaction, error) {
	return s.repo.ListByAccount(ctx, accountID)
}

// execute posts to the ledger, records the transaction and enqueues an event.
func (s *Service) execute(ctx context.Context, t domain.Transaction, entries []ledgerclient.Entry) (domain.Transaction, error) {
	t.ID = uuid.NewString()
	t.Status = domain.StatusPosted
	t.CreatedAt = time.Now().UTC()

	if _, _, err := s.ledger.Post(ctx, t.ID, t.IdempotencyKey, t.Currency, t.Reference, entries); err != nil {
		return domain.Transaction{}, ledgerErr(err)
	}

	var event *outbox.Event
	if s.emitEvents {
		payload, _ := json.Marshal(t)
		event = &outbox.Event{
			ID: uuid.NewString(), Source: s.source, Topic: EventTopic,
			Key: primaryAccount(t), Payload: payload,
		}
	}
	if err := s.repo.Create(ctx, t, event); err != nil {
		// Ledger already moved the money. Reverse it so a failed local write
		// cannot leave an unrecorded posting.
		rev := reverse(entries)
		if _, _, rerr := s.ledger.Post(ctx, uuid.NewString(), "reverse-"+t.ID, t.Currency, "reverse:"+t.ID, rev); rerr != nil {
			s.log.Error("ledger compensation failed", zap.String("transaction_id", t.ID), zap.Error(rerr))
		}
		return domain.Transaction{}, err
	}
	return t, nil
}

func (s *Service) replay(ctx context.Context, idemKey string) (domain.Transaction, bool) {
	if idemKey == "" {
		return domain.Transaction{}, false
	}
	existing, err := s.repo.GetByIdempotencyKey(ctx, idemKey)
	if err != nil {
		return domain.Transaction{}, false
	}
	return existing, true
}

func reverse(entries []ledgerclient.Entry) []ledgerclient.Entry {
	out := make([]ledgerclient.Entry, len(entries))
	for i, e := range entries {
		dir := "DEBIT"
		if e.Direction == "DEBIT" {
			dir = "CREDIT"
		}
		out[i] = ledgerclient.Entry{AccountID: e.AccountID, Direction: dir, Amount: e.Amount}
	}
	return out
}

func ledgerErr(err error) error {
	if err == nil {
		return apierror.New(502, "ledger_unavailable", "ledger service unavailable")
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "insufficient") {
		return apierror.New(422, "insufficient_funds", "insufficient funds")
	}
	return apierror.New(502, "ledger_unavailable", "ledger service unavailable")
}

func parsePositive(s string) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, apierror.ErrBadRequest("invalid amount")
	}
	if amount.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, apierror.ErrBadRequest("amount must be positive")
	}
	return amount, nil
}

func primaryAccount(t domain.Transaction) string {
	if t.FromAccountID != "" {
		return t.FromAccountID
	}
	return t.ToAccountID
}
