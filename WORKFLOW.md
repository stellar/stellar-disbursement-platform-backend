# SAPCONE Custom Disbursement Workflow

## Overview

This document describes the custom three-step role-based disbursement workflow implemented for SAPCONE on top of the Stellar Disbursement Platform (SDP).

## Workflow

The workflow consists of three distinct steps with role-based access control:

```
Uploader → Approver → FinanceOfficer
    │           │            │
    ▼           ▼            ▼
  READY      APPROVED      STARTED
```

### Step 1: Uploader Uploads CSV (READY)
- **Role**: `Uploader` (also allowed: `Initiator`, `Owner`, `FinancialController`)
- **Action**: POST `/api/v1/disbursements` - Creates a disbursement draft and uploads CSV instructions
- **Status Transition**: `DRAFT` → `READY`
- **Description**: The Uploader creates a disbursement by providing CSV data with receiver information. The disbursement enters `READY` status awaiting approval.

### Step 2: Approver Approves (APPROVED)
- **Role**: `Approver` (also allowed: `Owner`, `FinancialController`)
- **Action**: PATCH `/api/v1/disbursements/{id}/approve` - Approves the ready disbursement
- **Status Transition**: `READY` → `APPROVED`
- **Description**: The Approver reviews the disbursement details and approves it for submission. The approver cannot be the same person who created/uploaded the disbursement (enforced when approval workflow is enabled).

### Step 3: FinanceOfficer Submits to Stellar (STARTED)
- **Role**: `FinanceOfficer` (also allowed: `Owner`, `FinancialController`)
- **Action**: PATCH `/api/v1/disbursements/{id}/submit` - Submits the approved disbursement to Stellar network
- **Status Transition**: `APPROVED` → `STARTED`
- **Description**: The FinanceOfficer submits the approved disbursement to the Stellar network for execution.

## Status Machine

The disbursement status machine now includes the new `APPROVED` state:

```
DRAFT ──→ READY ──→ APPROVED ──→ STARTED ──→ COMPLETED
  ▲       │         ▲              │
  │       ▼         │              ▼
  └───── READY ◄───┘           PAUSED
```

### Valid Transitions
| From | To | Trigger |
|------|-----|---------|
| DRAFT | READY | Upload instructions |
| READY | READY | Re-upload instructions |
| READY | APPROVED | Approver approves |
| READY | STARTED | Start directly (approval not required) |
| APPROVED | READY | Approver rejects |
| APPROVED | STARTED | FinanceOfficer submits |
| STARTED | PAUSED | Pause disbursement |
| PAUSED | STARTED | Resume disbursement |
| STARTED | COMPLETED | All payments successful |

## API Endpoints

### 1. Create Disbursement (Uploader)
```
POST /api/v1/disbursements
```
**Allowed Roles**: `Uploader`, `Initiator`, `Owner`, `FinancialController`

**Request Body**:
```json
{
  "name": "Q3 Payroll",
  "asset_id": "asset-uuid",
  "wallet_id": "wallet-uuid",
  "registration_contact_type": "EMAIL",
  "receiver_registration_message_template": "Welcome to {{organization}}"
}
```

### 2. Approve Disbursement (Approver)
```
PATCH /api/v1/disbursements/{id}/approve
```
**Allowed Roles**: `Approver`, `Owner`, `FinancialController`

**Response**:
```json
{
  "message": "Disbursement approved"
}
```

**Errors**:
- `404`: Disbursement not found
- `400`: Disbursement not in READY status
- `403`: Approver cannot be the creator/uploader (when approval workflow enabled)

### 3. Submit Disbursement (FinanceOfficer)
```
PATCH /api/v1/disbursements/{id}/submit
```
**Allowed Roles**: `FinanceOfficer`, `Owner`, `FinancialController`

**Response**:
```json
{
  "message": "Disbursement submitted to Stellar"
}
```

**Errors**:
- `404`: Disbursement not found
- `400`: Disbursement not in APPROVED status
- `400`: Wallet disabled
- `409`: Insufficient balance

## Role Definitions

| Role | Description | Permissions |
|------|-------------|-------------|
| `Owner` | Full access | All operations |
| `FinancialController` | Financial operations | All except user management |
| `Developer` | Configuration only | Wallets, assets, statistics |
| `Business` | Read-only | View only (no user management) |
| `Initiator` | Create/save disbursements | POST /disbursements (no submit) |
| `Approver` | Submit disbursements | PATCH /disbursements/{id}/approve |
| `Uploader` | Upload CSV drafts | POST /disbursements |
| `FinanceOfficer` | Submit to Stellar | PATCH /disbursements/{id}/submit |

## Approval Workflow Configuration

Organizations can enable/disable the approval workflow:

```sql
UPDATE organizations SET is_approval_required = true;
```

When **enabled** (`is_approval_required = true`):
- READY → APPROVED → STARTED (three steps required)
- Approver cannot be the same as Uploader/Creator

When **disabled** (`is_approval_required = false`):
- READY → STARTED (direct transition allowed)
- Original single-step workflow preserved

## Database Changes

### Migration: `2026-07-16.0-add-approved-disbursement-status.sql`
```sql
-- +migrate Up
ALTER TYPE disbursement_status ADD VALUE IF NOT EXISTS 'APPROVED' AFTER 'READY';

-- +migrate Down
-- Note: PostgreSQL doesn't support removing enum values directly
-- ALTER TYPE disbursement_status DROP VALUE IF EXISTS 'APPROVED';
```

## Code Changes Summary

### Data Layer (`internal/data/`)
- `dibursements_state_machine.go`: Added `ApprovedDisbursementStatus`, new transitions
- `roles.go`: Added `UploaderUserRole` to `GetBusinessOperationRoles()`

### Services (`internal/services/`)
- `disbursement_management_service.go`: 
  - `ApproveDisbursement()` - READY → APPROVED
  - `SubmitDisbursement()` - APPROVED → STARTED
  - Updated `StartDisbursement()` for backward compatibility

### Handlers (`internal/serve/httphandler/`)
- `disbursement_handler.go`:
  - `ApproveDisbursement()` handler
  - `SubmitDisbursement()` handler

### Routes (`internal/serve/serve.go`)
- POST `/disbursements` - Uploader, Initiator, Owner, FinancialController
- PATCH `/disbursements/{id}/approve` - Approver, Owner, FinancialController
- PATCH `/disbursements/{id}/submit` - FinanceOfficer, Owner, FinancialController

## Testing

All tests pass:
- State machine transitions (including new APPROVED state)
- Handler endpoints for approve/submit
- Service layer validation
- Role-based access control
- Approval workflow enabled/disabled scenarios

Run tests:
```bash
go test ./internal/data/... -run "DisbursementStatus"
go test ./internal/serve/httphandler/... -run "Disbursement"
go test ./internal/services/... -run "DisbursementManagementService"
```

## Security Considerations

1. **Separation of Duties**: When approval workflow is enabled, the same user cannot both create/upload and approve a disbursement
2. **Role-Based Access**: Each step requires specific role permissions
3. **Status Validation**: State machine enforces valid transitions only
4. **Audit Trail**: All status changes recorded in `status_history` with user IDs