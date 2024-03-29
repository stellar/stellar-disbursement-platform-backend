# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## Unreleased

None

## [2.0.0.rc1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/2.0.0-rc1)

First Release Candidate of the Stellar Disbursement Platform v2.0.0. This
release introduces multi-tenancy support, allowing multiple tenants
(organizations) to use the platform simultaneously.

Each organization has its own set of users, receivers, disbursements, etc.

This version is only compatible with the [stellar/stellar-disbursement-platform-frontend] version 2.x.x.

### Changed
- Support multi-tenant CLI
  - Make `add-user` CLI support multi-tenancy [#228](https://github.com/stellar/stellar-disbursement-platform-backend/pull/228)
  - Change migrations CLI to run for all tenants [#89](https://github.com/stellar/stellar-disbursement-platform-backend/pull/89)
- Use DB connection pool as dependency injection [#207](https://github.com/stellar/stellar-disbursement-platform-backend/pull/207)
- Make receiver registration handler tenant-aware [#117](https://github.com/stellar/stellar-disbursement-platform-backend/pull/117)
- Tag log entries with tenant metadata [#192](https://github.com/stellar/stellar-disbursement-platform-backend/pull/192)
- Use `DistributionAccountResolver` instead of passing around distribution public key [#212](https://github.com/stellar/stellar-disbursement-platform-backend/pull/212)
- Make provision new tenant an atomic operation [#233](https://github.com/stellar/stellar-disbursement-platform-backend/pull/233)
- Make `ready_payments_cancellation` job multi-tenant [#223] (https://github.com/stellar/stellar-disbursement-platform-backend/pull/223)


### Added
- Tenant Provisioning & Onboarding [#84](https://github.com/stellar/stellar-disbursement-platform-backend/pull/84)
- Tenant Authentication Middleware [#92](https://github.com/stellar/stellar-disbursement-platform-backend/pull/92)
- Multi-tenancy connection pool & data source manager [#86](https://github.com/stellar/stellar-disbursement-platform-backend/pull/86)
- Generate multitenant SEP-1 TOML file [#111](https://github.com/stellar/stellar-disbursement-platform-backend/pull/111)
- Prepare Signature Service & TSS to support Multi-tenancy
  - Add signature service with configurable distribution accounts [#174](https://github.com/stellar/stellar-disbursement-platform-backend/pull/174)
  - Aggregate all tx submission dependencies under `SubmitterEngine` [#165](https://github.com/stellar/stellar-disbursement-platform-backend/pull/165)
  - Add configurable signature service type [#160](https://github.com/stellar/stellar-disbursement-platform-backend/pull/160)
  - Allow signature service to be dependency-injected [#158](https://github.com/stellar/stellar-disbursement-platform-backend/pull/158)
  - Use dependency-injected signature service in `channel-account` CLI commands [#156](https://github.com/stellar/stellar-disbursement-platform-backend/pull/156)
- '/tenant' endpoint
  - Setup tenant server [#90](https://github.com/stellar/stellar-disbursement-platform-backend/pull/90)
  - `POST` Provision tenant endpoint [#97](https://github.com/stellar/stellar-disbursement-platform-backend/pull/97)
  - `GET` Tenant(s) API [#93](https://github.com/stellar/stellar-disbursement-platform-backend/pull/93)
  - `PATCH` Tenant API [#100](https://github.com/stellar/stellar-disbursement-platform-backend/pull/100)
- `add-tenant` CLI [#76](https://github.com/stellar/stellar-disbursement-platform-backend/pull/76)
- Patch incoming TSS events to Anchor platform [#134](https://github.com/stellar/stellar-disbursement-platform-backend/pull/134)
- Update DB structure so that TSS resources can be shared by multiple SDP tenants
  - Move all TSS related tables to TSS schema [#141](https://github.com/stellar/stellar-disbursement-platform-backend/pull/141)
  - Create TSS schema and migrations CLI command [#136](https://github.com/
    stellar/stellar-disbursement-platform-backend/pull/136)
  - Refactor migrations commands to support TSS migrations [#123](https://github.com/stellar/stellar-disbursement-platform-backend/pull/123)
- Add host distribution account awareness [#172](https://github.com/stellar/stellar-disbursement-platform-backend/pull/172)
- Wire distribution account to tenant admin table during user provisioning [#198](https://github.com/stellar/stellar-disbursement-platform-backend/pull/198)
- Prepare transaction submission table to reference tenant [#142](https://github.com/stellar/stellar-disbursement-platform-backend/pull/142)
- Kafka message broker support
  - Migrate SMS invitation to use message broker from scheduled jobs [#133](https://github.com/stellar/stellar-disbursement-platform-backend/pull/133)
  - Publish receiver wallet invitation events at disbursement start [#182](https://github.com/stellar/stellar-disbursement-platform-backend/pull/182)
  - Produce payment events to sync back to SDP [#149] (https://github.com/stellar/stellar-disbursement-platform-backend/pull/149)
  - Produce payment events from SDP to TSS [#159](https://github.com/stellar/stellar-disbursement-platform-backend/pull/159)
- Implement `DistributionAccountDBSignatureClient` [#197](https://github.com/stellar/stellar-disbursement-platform-backend/pull/197)
- Create tenant distribution account during provisioning [#224](https://github.com/stellar/stellar-disbursement-platform-backend/pull/224)
- Enable payments scheduler job as an alternative to using Kafka [#230](https://github.com/stellar/stellar-disbursement-platform-backend/pull/230)


### Security
- Admin API authentication/authorization [#201](https://github.com/stellar/stellar-disbursement-platform-backend/pull/201)
- Enable security protocols for Kafka
  - SASL auth [#162](https://github.com/stellar/stellar-disbursement-platform-backend/pull/162)
  - SSL auth [#226](https://github.com/stellar/stellar-disbursement-platform-backend/pull/226)

## [1.1.5](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.4...1.1.5)

### Fixed

- Trim whitespaces for all disbursement instruction fields during CSV upload to avoid duplication of data  [#211](https://github.com/stellar/stellar-disbursement-platform-backend/pull/211)

### Security

- Upgrade golang version to 1.22.1 for security reasons [#216](https://github.com/stellar/stellar-disbursement-platform-backend/pull/216)

## [1.1.4](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.3...1.1.4)

### Fixed

- Fix the insufficient balance validation by only considering payments with same asset of the disbursement being started [#202](https://github.com/stellar/stellar-disbursement-platform-backend/pull/202)

### Security

- Update `golang.org/x/crypto` version to v0.17.0 for security reasons [#202](https://github.com/stellar/stellar-disbursement-platform-backend/pull/202)

## [1.1.3](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.2...1.1.3)

### Fixed

- SEP24 registration flow not working properly when the phone number was not found in the DB [#187](https://github.com/stellar/stellar-disbursement-platform-backend/pull/187)
- Fix distribution account balance validation that fails when the intended asset is XLM [#186](https://github.com/stellar/stellar-disbursement-platform-backend/pull/186)

## [1.1.2](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.1...1.1.2)

### Fixed

- Re-add missing recaptcha script [#179](https://github.com/stellar/stellar-disbursement-platform-backend/pull/179)

## [1.1.1](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.0...1.1.1)

### Fixed

- TSS amount precision [#176](https://github.com/stellar/stellar-disbursement-platform-backend/pull/176)

## [1.1.0](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.0.1...1.1.0)

### Changed

- Change `POST /disbursements` to accept different verification types [#103](https://github.com/stellar/stellar-disbursement-platform-backend/pull/103)
- Change `SEP-24` Flow to display different verifications based on disbursement verification type [#116](https://github.com/stellar/stellar-disbursement-platform-backend/pull/116)
- Add sorting to `GET /users` endpoint [#104](https://github.com/stellar/stellar-disbursement-platform-backend/pull/104)
- Change read permission for receiver details to include business roles [#144](https://github.com/stellar/stellar-disbursement-platform-backend/pull/144)
- Add support for unique payment ID to disbursement instructions file as an optional field in `GET /payments/{id}` [#131](https://github.com/stellar/stellar-disbursement-platform-backend/pull/131)
- Add support for SMS preview & editing before sending a new disbursement [#146](https://github.com/stellar/stellar-disbursement-platform-backend/pull/146)
- Add metadata for users that created and started a disbursement in disbursement details `GET /disbursements`, `GET /disbursements/{id}` [#151](https://github.com/stellar/stellar-disbursement-platform-backend/pull/151)
- Update CI check to run the exhaustive validator [#163](https://github.com/stellar/stellar-disbursement-platform-backend/pull/163)
- Preload reCAPTCHA script in attempt to mitigate component loading issues upon login [#152](https://github.com/stellar/stellar-disbursement-platform-backend/pull/152)
- Validate distribution account balance before starting disbursement [#161](https://github.com/stellar/stellar-disbursement-platform-backend/pull/161)

### Added

- Support automatic cancellation of payments in `READY` status after a certain time period [#121](https://github.com/stellar/stellar-disbursement-platform-backend/pull/121)
- API endpoint for cancelling payments in `READY` status: `PATCH /payments/{id}/status` [#130](https://github.com/stellar/stellar-disbursement-platform-backend/pull/130)
- Use CI to make sure the helm README is up to date [#164](https://github.com/stellar/stellar-disbursement-platform-backend/pull/164)

### Fixed

- Verification DOB validation missing when date is in the future [#101](https://github.com/stellar/stellar-disbursement-platform-backend/pull/101)
- Support disbursements from two or more wallet providers to the same address [#87](https://github.com/stellar/stellar-disbursement-platform-backend/pull/87)
- [TSS] Stale channel account not cleared after switching distribution keys [#91](https://github.com/stellar/stellar-disbursement-platform-backend/pull/91)
- Make setup-wallets-for-network tests more flexible [#95](https://github.com/stellar/stellar-disbursement-platform-backend/pull/95)
- Make `POST /assets` idempotent [#122](https://github.com/stellar/stellar-disbursement-platform-backend/pull/122)
- Add missing space when building query [#121](https://github.com/stellar/stellar-disbursement-platform-backend/pull/121)

### Security

- Stellar Protocol 20 Horizon SDK upgrade [#107](https://github.com/stellar/stellar-disbursement-platform-backend/pull/107)
- Coinspect Issues:
  - Add "Secure Operation Manual" section and updated the code to enforce MFA and reCAPTCHA [#150](https://github.com/stellar/stellar-disbursement-platform-backend/pull/150)
  - Coinspect SDP-006 Weak password policy [#143](https://github.com/stellar/stellar-disbursement-platform-backend/pull/143)
  - Coinspect SDP-007: Log user activity when updating user info [#139](https://github.com/stellar/stellar-disbursement-platform-backend/pull/139)
  - Coinspect SDP-012 Enhance User Awareness for SMS One-Time Password (OTP) Usage [#138](https://github.com/stellar/stellar-disbursement-platform-backend/pull/138)

## [1.0.1](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.0.0...1.0.1)

### Changed

- Update log message for better debugging. [#125](https://github.com/stellar/stellar-disbursement-platform-backend/pull/125)

### Fixed

- Fix client_domain from the Viobrant Assist wallet. [#126](https://github.com/stellar/stellar-disbursement-platform-backend/pull/126)

## [1.0.0](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.0.0-rc2...1.0.0)

### Added

- API endpoints for managing Wallet Providers:
  - Add Wallet Providers. [#17](https://github.com/stellar/stellar-disbursement-platform-backend/pull/17)
  - Soft delete a Wallet Provider. [#19](https://github.com/stellar/stellar-disbursement-platform-backend/pull/19)
  - Patch a Wallet Provider's status. [#37](https://github.com/stellar/stellar-disbursement-platform-backend/pull/37)
- Introduced metrics and Grafana dashboard for monitoring payment transactions in TSS. [#21](https://github.com/stellar/stellar-disbursement-platform-backend/pull/21)
- TSS documentation. [#25](https://github.com/stellar/stellar-disbursement-platform-backend/pull/25)
- Phone number validation before sending OTP. [#38](https://github.com/stellar/stellar-disbursement-platform-backend/pull/38)
- Add Vibrant Assist RC to the list of supported wallets in pubnet [#43](https://github.com/stellar/stellar-disbursement-platform-backend/pull/43)
- Store Anchor Platform transaction ID in the database when registering a new receiver. [#44](https://github.com/stellar/stellar-disbursement-platform-backend/pull/44)
- Documentation for `CRASH_TRACKER_TYPE` env variable. [#49](https://github.com/stellar/stellar-disbursement-platform-backend/pull/49)
- Add a job to periodically sync the transaction status back to the Anchor Platform [#55](https://github.com/stellar/stellar-disbursement-platform-backend/pull/55)
- Introduce a retry mechanism for SMS invitations. [#60](https://github.com/stellar/stellar-disbursement-platform-backend/pull/60)
- Add proper error messages when receiver exceeds the maximum number of attempts to validate their PII. [#62](https://github.com/stellar/stellar-disbursement-platform-backend/pull/62)

### Changed

- Add validation and flags to countries dropdown during receiver registration. [#33](https://github.com/stellar/stellar-disbursement-platform-backend/pull/33)
- Update transaction worker to use Crash Tracker on failed transactions [#39](https://github.com/stellar/stellar-disbursement-platform-backend/pull/39)
- Increase the default maximum number of attempts for a receiver to validate their PII. [#56](https://github.com/stellar/stellar-disbursement-platform-backend/pull/56)
- Prevent users from deactivating their own accounts. [#58](https://github.com/stellar/stellar-disbursement-platform-backend/pull/58)
- Stop enforcing ECDSA only and allow any EC public/private keys at least as strong as EC256 [#61](https://github.com/stellar/stellar-disbursement-platform-backend/pull/61)
- Refactor SMS invitation service [#66](https://github.com/stellar/stellar-disbursement-platform-backend/pull/66)
  - Removed the environment variables `MAX_RETRIES` and `MIN_DAYS_BETWEEN_RETRIES`.
  - Added the environment variable `MAX_INVITATION_SMS_RESEND_ATTEMPTS` to control the maximum number of attempts to send an SMS invitation. The default value is 3.
- API Tweaks:
  - Change PATCH `/organization` endpoint to allow updating the SMS templates. [#47](https://github.com/stellar/stellar-disbursement-platform-backend/pull/47)
  - Add the ability to filter supported assets by wallets. [#35](https://github.com/stellar/stellar-disbursement-platform-backend/pull/35)
  - Add wallets filtering by `enabled` flag [#72](https://github.com/stellar/stellar-disbursement-platform-backend/pull/72)
  - Return SMS templates in `GET /organization` endpoint. [#63](https://github.com/stellar/stellar-disbursement-platform-backend/pull/63)

### Fixed

- Stellar.Expert URL in env-config.js for dev environment setup. [#34](https://github.com/stellar/stellar-disbursement-platform-backend/pull/34)
- Patch the correct transaction data fields in AnchorPlatform. [#40](https://github.com/stellar/stellar-disbursement-platform-backend/pull/40)
- Sep10 domain configuration for Vibrant wallet on Testnet. [#42](https://github.com/stellar/stellar-disbursement-platform-backend/pull/42)
- The SMS invitation link for XLM asset. [#46](https://github.com/stellar/stellar-disbursement-platform-backend/pull/46)

### Security

- Added application activity logs for account lifecycle, password management and user access patterns. [#29](https://github.com/stellar/stellar-disbursement-platform-backend/pull/29)

## [1.0.0.rc2](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.0.0-rc1...1.0.0-rc2)

### Added

- Support to XLM disbursements. [#1](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1)
- Helm chart documentation. [#9](https://github.com/stellar/stellar-disbursement-platform-backend/pull/9)
- `PATCH /profile/reset-password` endpoint to reset the password. [#18](https://github.com/stellar/stellar-disbursement-platform-backend/pull/18)

### Changed

- Helmchart changes:
  - (BREAKNG CHANGE) Refactor helmchart for consistency. [#5](https://github.com/stellar/stellar-disbursement-platform-backend/pull/5)
  - Add `minimal-values.yaml` file to the helm folder, so it becomes easier to configure it. [#20](https://github.com/stellar/stellar-disbursement-platform-backend/pull/20)
  - Update Helm charts to include the frontend dashboard as part of the release. [#3](https://github.com/stellar/stellar-disbursement-platform-backend/pull/3)
- Default `MAX_BASE_FEE` is now higher, to prevent low-fee error responses. [#8](https://github.com/stellar/stellar-disbursement-platform-backend/pull/8)
- Changed job frequency for more real-time updates. [#12](https://github.com/stellar/stellar-disbursement-platform-backend/pull/12)
- Change OTP message for better UX. [#23](https://github.com/stellar/stellar-disbursement-platform-backend/pull/23)
- API tweaks:
  - `GET /receiver/{id}` now returns the list of verification fields in the receiver object. [#4](https://github.com/stellar/stellar-disbursement-platform-backend/pull/4)
  - `GET /profile` now includes the user `id` in the json response. [#2](https://github.com/stellar/stellar-disbursement-platform-backend/pull/2)
  - Standardize 401 API responses [#15](https://github.com/stellar/stellar-disbursement-platform-backend/pull/15).
  - Changed the window in which the refresh token can be generated. [#7](https://github.com/stellar/stellar-disbursement-platform-backend/pull/7)

### Fixed

- TSS Channel Account management commands now can handle parallel calls. [#6](https://github.com/stellar/stellar-disbursement-platform-backend/pull/6)
- Horizon error parsing to use the default `HorizonErrorWrapper` class. [#13](https://github.com/stellar/stellar-disbursement-platform-backend/pull/13)
- API response that should be 401 instead of 500. [#14](https://github.com/stellar/stellar-disbursement-platform-backend/pull/14)

### Security

- Removed CLI flag that could disable private key encryption in the database. [$24](https://github.com/stellar/stellar-disbursement-platform-backend/pull/24)
- Add job to periodically check if the AP is auth protected. [#10](https://github.com/stellar/stellar-disbursement-platform-backend/pull/10)
- Add stronger password validation throughout the project. [#22](https://github.com/stellar/stellar-disbursement-platform-backend/pull/22)

## [1.0.0.rc1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/1.0.0-rc1)

### Added

First Release Candidate of the Stellar Disbursement Platform, a tool used to make bulk payments to a list of recipients
based on their phone number and a confirmation date. This repository is backend-only, and the frontend version can be
found at [stellar/stellar-disbursement-platform-frontend]. Their version numbers are meant to be kept in sync.

The basic process of this product starts with an organization supplying a CSV file which includes the recipients' phone
number, transfer amount, and essential customer validation data such as the date of birth.

The platform subsequently sends an SMS to the recipient, which includes a deep link to the wallet. This link permits
recipients with compatible wallets to register their wallet on the SDP. During this step, they are required to verify
their phone number and additional customer data through the SEP-24 interactive deposit flow, where this data is shared
directly with the backend through a webpage inside the wallet, but the wallet itself does not have access to this data.

Upon successful verification, the SDP will transfer the funds directly to the recipient's wallet. When the recipient's
wallet has been successfully associated with their phone number in the SDP, all subsequent payments will occur
automatically.

[stellar/stellar-disbursement-platform-frontend]: https://github.com/stellar/stellar-disbursement-platform-frontend
