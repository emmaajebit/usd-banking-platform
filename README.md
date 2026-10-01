# USD Banking Platform

Learning bank for a single currency: USD. A customer can register, open an account, deposit, withdraw, transfer, spend on a debit card, and repay a loan. Every balance change posts through a double-entry ledger. A customer account cannot go below zero.

This repository is for learning. It is not a licensed bank, and the default secret is not safe to share.

## Run the customer journey

Requires Docker with Compose.

```sh
git clone https://github.com/emmaajebit/usd-banking-platform.git
cd usd-banking-platform
docker compose -f docker-compose.yaml -f docker-compose.core.yaml up --build
```

| Surface | URL |
| --- | --- |
| Web app | http://localhost:5173 |
| API gateway | http://localhost:8080 |
| Postgres | localhost:5432 (`banking` / `banking`, database `banking`) |
| Redis | localhost:6379 |
| Kafka (from the host) | localhost:9092 |
| Kafka UI | http://localhost:8090 |

Check the money path after the gateway is ready:

```sh
BASE=http://localhost:8080 ./scripts/smoke.sh
```

The script registers a user, opens a savings account, deposits 100.00, withdraws 40.00, and expects a balance of 60.00.

Stop the stack with `docker compose -f docker-compose.yaml -f docker-compose.core.yaml down`. Add `-v` to drop the database and start clean.

Operations, failure cases, and the cluster path are in [RUNBOOK.md](RUNBOOK.md).

## What the core stack does

`docker-compose.yaml` starts Postgres, Redis, and Kafka. `docker-compose.core.yaml` starts the services that move money:

| Service | Role |
| --- | --- |
| auth-service | Register, login, JWT |
| ledger-service | Double-entry source of truth, gRPC on 9090 |
| account-service | Opens a bank account and its ledger account |
| transaction-service | Deposit, withdraw, transfer; writes an outbox event |
| statement-service | Builds statement lines from `banking.transactions` |
| notification-service | Stores a notice per consumed event |
| card-service | Issue, block, and debit-card spend |
| loan-service | Apply, disburse, repay |
| api-gateway | Authenticates and proxies `/api/v1/*` |
| frontend | React app for the journey above |

The other services under `services/` are part of the wider platform layout (KYC, documents, reports, and so on). They are not required for the USD journey.

## Money rules

- Only USD is accepted.
- A credit increases a customer balance. A debit decreases it. Every posting balances.
- System cash is the other side of a deposit, withdrawal, or card spend. Loan funding is the other side of a disbursement or repayment. Those two accounts may go negative.
- The ledger locks the affected accounts and is the only place that rejects an overdraft. Callers do not check the balance and then post.
- Repeating the same `Idempotency-Key` does not post twice.
- If the ledger posts and the local transaction row fails, transaction-service writes a reversing entry.

Opening an account opens the ledger account first. Ledger startup seeds the USD system cash and loan-funding accounts.

## API sketch

All routes except register and login need `Authorization: Bearer <access_token>`.

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/accounts                      {"type":"SAVINGS","currency":"USD"}
GET  /api/v1/accounts/:id/balance
POST /api/v1/transactions/deposit
POST /api/v1/transactions/withdraw
POST /api/v1/transactions/transfer
POST /api/v1/cards                         {"account_id","type":"DEBIT"}
POST /api/v1/cards/:id/spend               {"amount":"10.00"}
POST /api/v1/loans
POST /api/v1/loans/:id/approve             staff role
POST /api/v1/loans/:id/disburse            staff role
POST /api/v1/loans/:id/repay               {"amount":"100.00"}
```

Money writes should send `Idempotency-Key`.

## Layout

```text
services/          one Go module per service
pkg/               shared config, HTTP, gRPC, Kafka, outbox, system accounts
proto/             gRPC contracts
frontend/          React 18 + Vite
deploy/helm/       umbrella chart
deploy/argocd/     GitOps apps
terraform/         EKS, VPC, ECR, IAM modules
docker-compose*.yaml
scripts/smoke.sh
```

Shared Postgres uses a table prefix per service. Kafka topic for posted transactions is `banking.transactions`.

## Tests

With Go 1.25 or newer:

```sh
go test ./pkg/systemacct/ ./services/ledger-service/internal/repository/
```

`make test` runs the workspace modules inside Docker. `make compose-up` starts only Postgres, Redis, and Kafka.

## Cluster

GitHub Actions builds images and can bump Helm tags. Argo CD applies the chart. Nothing in this repo deploys a cluster by itself. Set `JWT_SECRET` from a Kubernetes secret before more than one person can reach the API. See [RUNBOOK.md](RUNBOOK.md).
