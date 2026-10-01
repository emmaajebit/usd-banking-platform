# Live platform changes

This branch turns the portfolio skeleton into a USD customer journey that posts every money movement through the ledger.

## What changed

- Ledger postings lock the affected accounts and reject a customer balance below zero. System cash and loan-funding accounts may go negative; they are the contra side.
- Deposits, withdrawals, and transfers no longer check funds in a separate call. The ledger is the only place that decides.
- If the ledger posts and the transaction row fails, the service writes a reversing entry.
- Opening an account still opens the ledger account first, and only USD is accepted.
- Ledger startup seeds the USD system cash and loan-funding accounts.
- Debit card spend posts a customer debit and a system-cash credit.
- Loan repayment posts a customer debit and a funding-account credit, then reduces outstanding principal.
- Dashboard shows ledger balances. Cards and loans can spend and repay from the UI.
- `docker-compose.core.yaml` runs the journey services. `scripts/smoke.sh` checks register, deposit, withdraw, and a 60.00 balance.

## Run locally

```sh
docker compose -f docker-compose.yaml -f docker-compose.core.yaml up --build
# app: http://localhost:5173   api: http://localhost:8080
BASE=http://localhost:8080 ./scripts/smoke.sh
```

EKS deploys the same images through the existing GitHub Actions and Argo CD path. Set `JWT_SECRET` from a Kubernetes secret before treating the cluster as shared.
