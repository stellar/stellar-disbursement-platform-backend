# Multi-Distribution-Account - API Reference

The endpoints added by multi-distribution-account support. All live under `/distribution-wallets`
and are reachable with a JWT or an API key.

## Authorization

Two new API-key permissions were added: `read:distribution_wallets` and `write:distribution_wallets`
(note the underscore; the URL path is hyphenated). `read:all` and `write:all` also grant them.

Most of these endpoints are Owner-only. That check binds both authentication paths, but it means
something different for each:

- **JWT**: the signed-in user must be an Owner.
- **API key**: the *user who created the key* must be an Owner. A key minted by a Developer is
  rejected even when it carries `write:distribution_wallets`. Deactivating or deleting a key's
  creator withdraws that key from these endpoints.

Reads that are not Owner-only are scoped instead: Owners see every account, everyone else sees only
the accounts they have been granted access to. An account outside that scope returns `404`, never
`403`, so existence is never disclosed.

## Endpoints

| Method | Path | Access |
| :-- | :-- | :-- |
| `GET` | `/distribution-wallets` | Any role, scoped |
| `POST` | `/distribution-wallets` | Owner |
| `GET` | `/distribution-wallets/balance` | Any role, scoped |
| `GET` | `/distribution-wallets/{id}` | Owner |
| `GET` | `/distribution-wallets/{id}/balance` | Any role, scoped |
| `GET` | `/distribution-wallets/{id}/capabilities` | Any role, scoped |
| `GET` | `/distribution-wallets/{id}/memberships` | Owner |
| `POST` | `/distribution-wallets/{id}/memberships` | Owner |
| `DELETE` | `/distribution-wallets/{id}/memberships/{membershipID}` | Owner |
| `GET` | `/distribution-wallets/{id}/audit` | Owner |
| `POST` | `/distribution-wallets/{id}/archive` | Owner |
| `POST` | `/distribution-wallets/{id}/promote-to-default` | Owner |

### The distribution account object

```json
{
  "id": "8d1f6a2e-3c4b-4a5d-9e6f-7a8b9c0d1e2f",
  "name": "payroll",
  "description": "Monthly payroll runs",
  "distribution_account_address": "GA7Q...VSGZ",
  "distribution_account_type": "DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT",
  "distribution_account_status": "ACTIVE",
  "status": "ACTIVE",
  "is_default": true,
  "created_at": "2026-08-01T10:00:00Z",
  "updated_at": "2026-08-01T10:00:00Z"
}
```

The object carries two separate status fields:

- `status` is the account's lifecycle inside the SDP: `ACTIVE`, `PENDING` or `ARCHIVED`. A `PENDING`
  account has been reserved but not yet funded on-chain and cannot send. `archived_at` is present
  only once archived.
- `distribution_account_status` is the readiness of the underlying account itself, mirrored from the
  tenant: `ACTIVE`, or `PENDING_USER_ACTIVATION` for a Circle tenant that has not yet supplied its
  credentials. Stellar accounts are provisioned automatically and are always `ACTIVE`.

`distribution_account_address` is absent until the account is provisioned.

### List accounts

`GET /distribution-wallets` returns an array of distribution accounts. Archived accounts are
included so historical records stay resolvable.

### Create an account

`POST /distribution-wallets` — Owner only. Returns `201` with the new account.

```json
{ "name": "payroll", "description": "Monthly payroll runs" }
```

The account is generated, funded and given its own signing key. Only tenants on
`DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT` are eligible, with a maximum of 20 accounts.

- `400` — name missing, account cap reached, unsupported account type, or tenant not eligible
- `409` — an account with that name already exists

### Balances

`GET /distribution-wallets/{id}/balance` returns one account's live on-chain balances.
`GET /distribution-wallets/balance` returns the combined total across every account the caller can
see, and omits `wallet_id`.

```json
{
  "wallet_id": "8d1f6a2e-3c4b-4a5d-9e6f-7a8b9c0d1e2f",
  "balances": {
    "USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5": "12500.0000000",
    "XLM": "104.9999200"
  }
}
```

Balances are keyed `CODE:ISSUER`, except the native asset which is keyed `XLM`.

### Capabilities

`GET /distribution-wallets/{id}/capabilities` reports what the caller may actually do on one
account, combining their tenant-wide role with the role they hold on that account. Both gates
apply, which is why this is a capability set rather than a single effective role.

```json
{
  "wallet_id": "8d1f6a2e-3c4b-4a5d-9e6f-7a8b9c0d1e2f",
  "capabilities": {
    "can_create_disbursement": true,
    "can_start_disbursement": false,
    "can_pause_disbursement": false,
    "can_cancel_disbursement": false,
    "can_create_payment": true,
    "can_retry_payment": true,
    "can_cancel_payment": false
  }
}
```

Owners can ask what a third party could do by passing `?user_id=`, optionally with `?role=` to test
a role the user does not hold yet. `role` on its own is rejected with `400` — it always needs a
`user_id` alongside it. The response echoes whichever was supplied. This backs the dashboard's
access-granting picker, which warns when a grant would be inert because the user's tenant-wide role
already excludes the action.

### Access management

`GET /distribution-wallets/{id}/memberships` lists who has access to an account.

`POST /distribution-wallets/{id}/memberships` grants access. Returns `201`.

```json
{ "user_id": "…", "role": "financial_controller" }
```

- `400` — `user_id` missing, or the role is not wallet-scopable
- `409` — the user already holds that role, or the account is archived

The `owner` role cannot be granted here: Owner is always tenant-wide.

`DELETE /distribution-wallets/{id}/memberships/{membershipID}` revokes access and returns `204`. The
grant history is preserved in the audit trail.

`GET /distribution-wallets/{id}/audit` returns the grants and revokes recorded against an account,
newest first, from an append-only table. Each entry is a membership plus `operation` and
`changed_at`. Archived accounts remain queryable. The response is capped at the 100 most recent
entries and is not paginated, so older history has to be read from the database directly.

### Lifecycle

`POST /distribution-wallets/{id}/archive` archives an account. Archived accounts accept no new
disbursements but keep serving reads.

- `400` — the account is the tenant's default, or the last active account
- `404` — not found, or already archived

`POST /distribution-wallets/{id}/promote-to-default` makes an account the tenant's default, demoting
the previous one atomically.

- `400` — the account is not active
- `404` — not found

## The `X-Wallet-Id` header

Multi-account support also adds a request header to existing endpoints. It selects which
distribution account a request acts on, and it narrows what a request can reach — never widens it.
Naming an account the caller has no access to reaches nothing.

On reads it is optional. If sent, results cover that account; if omitted, they cover every account
the caller can reach.

On writes that send funds or create records against an account, it names the source account and is
**required** once a tenant has more than one active account — `POST /disbursements`, `POST /payments`,
`POST /receivers`, and asset/trustline writes return `400` without it. Single-account tenants are
unaffected, and an API key scoped to exactly one account may omit it.

## API keys

`distribution_wallet_ids` was added to the API key endpoints (`POST /api-keys`, `PATCH /api-keys/{id}`,
and the key responses). Omitting it on creation inherits the accounts its creator can reach at that
moment; the resolved IDs are stored on the key, so accounts added afterwards are not included. An
empty array grants no account access at all.
