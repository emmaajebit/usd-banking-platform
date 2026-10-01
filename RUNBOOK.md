# Runbook

How to start, check, and reset the USD banking platform. Use this for the local journey first. The cluster section is the path after you have your own AWS account and cluster.

## 1. Local start

Prerequisites: Docker Compose v2, `curl`, and `jq` for the smoke script.

```sh
docker compose -f docker-compose.yaml -f docker-compose.core.yaml up --build
```

Wait until `api-gateway` answers and the frontend is serving.

```sh
curl -fsS http://localhost:8080/healthz
curl -fsS -o /dev/null -w '%{http_code}\n' http://localhost:5173
```

Default local secret is `dev-secret-change-me`. Every service in the compose file must use the same `JWT_SECRET`. Postgres user, password, and database are all `banking`.

## 2. Smoke the money path

```sh
BASE=http://localhost:8080 ./scripts/smoke.sh
```

Expected last lines:

```text
balance=60.00
smoke ok
```

`60` is also accepted. Any other balance means a posting was skipped or applied twice. Do not rerun the script against the same idempotency keys and expect a second deposit. The script creates a new user each run, so a second run is a new account.

## 3. Manual journey

1. Open http://localhost:5173 and register.
2. Open a USD savings account. The API rejects any other currency.
3. Deposit, then withdraw or transfer. The dashboard reads the balance from the ledger, not from the account row.
4. Issue a debit card and spend. The spend debits the linked account.
5. Apply for a loan. Approve and disburse need a staff role. Repay is available once the loan is `DISBURSED`.

Staff routes are `POST /api/v1/loans/:id/approve` and `POST /api/v1/loans/:id/disburse`. A normal customer token receives 403.

## 4. Where to look when something fails

| Symptom | Check |
| --- | --- |
| Gateway 502 on `/api/v1/accounts` | `account-service` is up and `LEDGER_GRPC_ADDR=ledger-service:9090` |
| Open account returns `ledger_unavailable` | ledger gRPC is not listening, or the account id was not created in the ledger |
| Withdraw returns `insufficient_funds` | Expected. The ledger rejected the debit. Deposit first. |
| Balance stays 0 after a deposit | Transaction row may exist while the ledger post failed, or you are reading a different account id |
| Statement page empty | `statement-service` must be consuming Kafka. Deposit only writes the statement after the outbox publisher runs |
| Notices empty on the dashboard | Notification events are keyed by account id, not user id, so the user query can be empty |
| Login works, other calls 401 | `JWT_SECRET` differs between auth-service and the service behind the gateway |
| Compose build fails on a service | Build from the repo root. The Dockerfile context is `.`, not the service directory |

Logs:

```sh
docker compose -f docker-compose.yaml -f docker-compose.core.yaml logs -f ledger-service transaction-service api-gateway
```

Health for a published service port from `docker-compose.services.yaml` is `http://localhost:81xx/healthz` and `/readyz`. The core file does not publish those ports. The gateway on 8080 is the public entry.

## 5. Reset

```sh
docker compose -f docker-compose.yaml -f docker-compose.core.yaml down -v
docker compose -f docker-compose.yaml -f docker-compose.core.yaml up --build
```

`-v` deletes Postgres, Redis, and Kafka data. The next ledger start seeds USD system cash and loan funding again at zero.

## 6. Run one service without the full stack

Dependencies only:

```sh
docker compose up -d
```

Then, with Go installed, from `services/<name>`:

```sh
POSTGRES_HOST=localhost POSTGRES_ENABLED=true JWT_SECRET=dev-secret-change-me \
  OTEL_ENABLED=false LOG_FORMAT=console go run ./cmd
```

Ledger must also set `GRPC_ENABLED=true`. Account, transaction, card, and loan need `LEDGER_GRPC_ADDR=localhost:9090`. Transaction, statement, and notification need `KAFKA_ENABLED=true` and `KAFKA_BROKERS=localhost:9092` when you want events.

`make compose-up` is the same dependency start. `make test` runs module tests in Docker.

## 7. Ledger incident notes

- Do not update `ledger_accounts.balance` by hand. Insert a balanced posting, or restore from a backup.
- A customer balance below zero is a defect. System cash and loan funding are allowed to be negative.
- Duplicate `Idempotency-Key` on the ledger is a no-op. A lost response can be retried safely.
- If transaction-service logs `ledger compensation failed`, the ledger moved money and the reversing entry did not apply. Compare `ledger_entries` with `transaction_transactions` for that id before retrying.

Useful queries in `banking`:

```sql
SELECT account_id, balance, currency FROM ledger_accounts ORDER BY created_at;
SELECT id, idempotency_key, reference FROM ledger_transactions ORDER BY created_at DESC LIMIT 20;
SELECT id, type, amount, status FROM transaction_transactions ORDER BY created_at DESC LIMIT 20;
```

## 8. Cluster path

This repo does not create a cluster on clone.

1. Copy `terraform/environments/dev` and set a remote state backend you own. Do not apply against an account you do not control.
2. Create the EKS cluster and ECR repository from those modules.
3. Put `JWT_SECRET` in a Kubernetes secret and mount the same value into every service. Do not leave `dev-secret-change-me` on a shared cluster.
4. GitHub Actions (`.github/workflows/ci.yaml`) builds images and can commit Helm tag bumps. Argo CD, not the workflow, applies the chart under `deploy/helm/banking-platform`.
5. Confirm ledger gRPC is reachable only inside the cluster. Account, transaction, card, and loan need that address.
6. Run the same smoke script against the ingress only after auth and the ledger are ready:

```sh
BASE=https://<your-host> ./scripts/smoke.sh
```

Roll back by reverting the image tag commit and letting Argo CD sync. Do not edit balances to undo a bad release. Post a reversing entry or restore the database.
