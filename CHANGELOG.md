# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## Unreleased

- Add sorting to `GET /users` endpoint [#104](https://github.com/stellar/stellar-disbursement-platform-backend/pull/104)

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
