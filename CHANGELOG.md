# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

## [6.2.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/6.2.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/6.1.0...6.2.0))

### Added

- Add integration tests for embedded wallets. [1041](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1041)
- Add endpoint for fetch captcha config. [1052](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1052)
- Implement reCAPTCHA v3 for receiver registration page. [1053](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1053)

### Changed

- Mirror CI checks in Makefile for local development parity. [#1070](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1070)

### Fixed

- Fix short linking is not enabled by default. [#1051](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1051)
- Fix Docker Compose build racing for e2e integration tests. [#1054](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1054)
- Make Circle Transfer Request Insert operation atomic. [#1050](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1050)
- Fix unbounded CSV upload size and pagination page_limit allowing resource exhaustion. [#1064](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1064)
- Check muxed account auth is not infra account in sponsored transaction worker. [#1068](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1068)
- Remove the default "false" value for ENABLE_EMBEDDED_WALLETS. [#1049](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1049)

### Security

- Add request body size limit to RPC proxy handler to prevent unbounded memory allocation (CWE-770). [#1065](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1065)
- Bump golang from 1.26rc2-alpine to 1.26rc3-alpine in the all-docker group [#1047](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1047)
- Bump docker/build-push-action from 6.18.0 to 6.19.2 in the all-actions group [#1056](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1056)
- Bump the cargo group across 1 directory with 2 updates [#1067](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1067)
- Bump the minor-and-patch group across 1 directory with 10 updates [#1069](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1069)
- Bump time from 0.3.41 to 0.3.47 in /contracts in the cargo group [#1045](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1045)

### Security

- Add global `MaxBodySize` middleware (10 MB) to all routes on both the SDP and admin servers to prevent unbounded request body sizes (CWE-770). [#1066](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1066)

## [6.1.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/6.1.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/6.0.1...6.1.0))

### Fixed

-  Fix PatchDisbursementStatus to support API key authentication. [#996](https://github.com/stellar/stellar-disbursement-platform-backend/pull/996)

### Added

- Add Haitian Creole translations for the SEP-24 interactive deposit flow. [#994](https://github.com/stellar/stellar-disbursement-platform-backend/pull/994)
- Return `sender_address` in `GET /payments` and `GET /payments/:id` endpoints to identify the distribution account used for each payment. [#1009](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1009)
- Add embedded wallet provider [#1019](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1019)

### Changed

- Update `GET /wallets` endpoint to exclude soft-deleted wallets by default. Add optional `include_deleted` query parameter to include deleted wallets when set to `true`. [#1005](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1005)
- Update `DELETE /wallets/:id` endpoint to check if a wallet has pending registrations before deletion. Returns a user-friendly error if the wallet has receiver_wallets in 'DRAFT' or 'READY' status. [#1007](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1007)
- Improve error handling for disbursement field `id` and `paymentId`. [#1018](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1018)
- Soft delete "Vibrant Assist" wallet provider. [#1021](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1021)

### Security and Dependencies

- Upgrade React to 19.2.3 and @stellar/design-system to 3.2.7 in SEP-24 frontend to address CVE-2025-55184 denial of service and source code exposure vulnerability in React Server Components [#988](https://github.com/stellar/stellar-disbursement-platform-backend/pull/988)
- Bump soroban-sdk from 22.0.7 to 22.0.9 in /contracts [#1036](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1036)
- Bump lodash from 4.17.21 to 4.17.23 in /internal/serve/sep24frontend/app [#1022](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1022)
- Bump golang from 1.25.5-alpine to 1.26rc2-alpine in the all-docker group [#1013](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1013)
- Bump the all-actions group across 1 directory with 5 updates [#1038](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1038)
- Bump the minor-and-patch group with 5 updates [#1012](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1012)
- Bump the minor-and-patch group with 7 updates [#1004](https://github.com/stellar/stellar-disbursement-platform-backend/pull/1004)
- Bump the minor-and-patch group with 8 updates [#990](https://github.com/stellar/stellar-disbursement-platform-backend/pull/990)
- Bump the minor-and-patch group with 3 updates [#993](https://github.com/stellar/stellar-disbursement-platform-backend/pull/993)

## [6.0.1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/6.0.1) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/6.0.0...6.0.1))

### Fixed

- Fix Twilio error 21656 when sending WhatsApp OTP messages by updating template to single variable format as required by Twilio/Meta's updated authentication template requirements [#974](https://github.com/stellar/stellar-disbursement-platform-backend/pull/974)

### Security and Dependencies

- Bump the all-docker group with 2 updates [#976](https://github.com/stellar/stellar-disbursement-platform-backend/pull/976)
  - golang from 1.25.4-alpine to 1.25.5-alpine
  - alpine from 3.22 to 3.23
- Bump the minor-and-patch group with 7 updates [#977](https://github.com/stellar/stellar-disbursement-platform-backend/pull/977)

## [6.0.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/6.0.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/5.0.0...6.0.0))

### 🚨Potential Breaking Changes

- Remove case insensitivity from asset code comparisons [#967](https://github.com/stellar/stellar-disbursement-platform-backend/pull/967)
- Decommission Anchor Platform and add support for SEP-10 and SEP-24 endpoints in the SDP [#834](https://github.com/stellar/stellar-disbursement-platform-backend/pull/834)
    - Add SEP10 /auth endpoints
    - Add SEP24 /info endpoints
    - Add SEP24 interactive deposit endpoint
    - Remove Anchor Platform integration from the SDP and deprecate related env variables.

### Added

- Add Launch Wizard through `make setup` command to simplify initial setup and mainnet configuration for docker compose [#875](https://github.com/stellar/stellar-disbursement-platform-backend/pull/875)
    - Add `--env-file` command line argument to support multiple environment file contexts [#963](https://github.com/stellar/stellar-disbursement-platform-backend/pull/963) 
- Add HTTPS mode to setup wizard [#957](https://github.com/stellar/stellar-disbursement-platform-backend/pull/957)
- Add configurable database connection pool settings to prevent idle connection buildup in multi-tenant deployments [#932](https://github.com/stellar/stellar-disbursement-platform-backend/pull/932)

### Changed

- Default Max Open Connections per pool changed from 30 to 20 to prevent idle connection buildup in multi-tenant deployments [#932](https://github.com/stellar/stellar-disbursement-platform-backend/pull/932)
- `channel-accounts ensure` command now uses batch account deletion to improve efficiency. [#938](https://github.com/stellar/stellar-disbursement-platform-backend/pull/938)
- Make docker compose environment variables configurable via `.env` file and add documentation [#953](https://github.com/stellar/stellar-disbursement-platform-backend/pull/953)
- Update Stellar Go SDK dependency from `github.com/stellar/go` to `github.com/stellar/go-stellar-sdk` [#956](https://github.com/stellar/stellar-disbursement-platform-backend/pull/956)

### Fixed

- Fix HTML validation to allow apostrophes in invitation messages while maintaining security against XSS attacks [#931](https://github.com/stellar/stellar-disbursement-platform-backend/pull/931)
- Refactor to replace float64 with shopspring decimal [#936](https://github.com/stellar/stellar-disbursement-platform-backend/pull/936)
- `channel-accounts ensure` command should have a minimum of 1 [#939](https://github.com/stellar/stellar-disbursement-platform-backend/pull/939)

### Security and Dependencies

- Upgrade React to 19.2.1 in SEP-24 frontend to address CVE-2025-66478 and CVE-2025-55182 [#968](https://github.com/stellar/stellar-disbursement-platform-backend/pull/968)
- Bump the minor-and-patch group with 5 updates [#961](https://github.com/stellar/stellar-disbursement-platform-backend/pull/961)
- Bump the minor-and-patch group across 1 directory with 12 updates [#955](https://github.com/stellar/stellar-disbursement-platform-backend/pull/955)
- Bump actions/checkout from 5 to 6 in the all-actions group [#954](https://github.com/stellar/stellar-disbursement-platform-backend/pull/954)
- Bump js-yaml from 4.1.0 to 4.1.1 in the npm_and_yarn group [#947](https://github.com/stellar/stellar-disbursement-platform-backend/pull/947)
- Bump golang from 1.25.3-alpine to 1.25.4-alpine in the all-docker group [#940](https://github.com/stellar/stellar-disbursement-platform-backend/pull/940)
- Bump the minor-and-patch group with 7 updates [#933](https://github.com/stellar/stellar-disbursement-platform-backend/pull/933)

## [5.0.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/5.0.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/4.1.0...5.0.0))

### Added

- Improve observability for the SDP service by adding the following : 
  - tag metrics by tenant name to differentiate between tenants
  - configure Summary metrics with percentiles for HTTP request durations (0.5, 0.9, 0.95, 0.99)
  - rework Grafana dashboard to include tenant tag and new metrics
  - [#818](https://github.com/stellar/stellar-disbursement-platform-backend/pull/818)
- Add organization level MFA and ReCAPTCHA settings [#861](https://github.com/stellar/stellar-disbursement-platform-backend/pull/861)
- Add trustlines for distribution account when provisioning tenant [#891](https://github.com/stellar/stellar-disbursement-platform-backend/pull/891)
- Add support for contract account disbursements [#922](https://github.com/stellar/stellar-disbursement-platform-backend/pull/922)
- Add contract account support for direct payments [#924](https://github.com/stellar/stellar-disbursement-platform-backend/pull/924)
- Add support for contract addresses for PATCH receiver [#925](https://github.com/stellar/stellar-disbursement-platform-backend/pull/925)
- Mark tx failures due to archived entries as error [#926](https://github.com/stellar/stellar-disbursement-platform-backend/pull/926)

### Changed
- Decommissioned Event Broker Kafka support in favor of Scheduler for background jobs. [#914](https://github.com/stellar/stellar-disbursement-platform-backend/pull/914)
- Allow configuring `resources` limits and requests for services in the Helm charts [#904](https://github.com/stellar/stellar-disbursement-platform-backend/pull/904)
- Enable short linking by default [#916](https://github.com/stellar/stellar-disbursement-platform-backend/pull/916)
- Make POST /wallets and PATCH /wallets permissions consistent [#909](https://github.com/stellar/stellar-disbursement-platform-backend/pull/909)

## [4.1.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/4.1.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/4.0.1...4.1.0))

### Added

- Add Support For Twilio WhatsApp messaging [#855](https://github.com/stellar/stellar-disbursement-platform-backend/pull/855)
- Add Twilio WhatsApp template documentation. [#877](https://github.com/stellar/stellar-disbursement-platform-backend/pull/877)
- Added initiator and approver user roles with mutual exclusivity validation for separation of duties in disbursement workflows. [#865](https://github.com/stellar/stellar-disbursement-platform-backend/pull/865)
- Ability to Onboard existing customers to Bridge integration via `PATCH /bridge-integration` endpoint. [#867](https://github.com/stellar/stellar-disbursement-platform-backend/pull/867)
- Add endpoint for patch receiver wallet. [#848](https://github.com/stellar/stellar-disbursement-platform-backend/pull/848)
- Add ReCAPTCHA v3 support. [#869](https://github.com/stellar/stellar-disbursement-platform-backend/pull/869)
- Enable direct payments to SEP-24 wallets. [#860](https://github.com/stellar/stellar-disbursement-platform-backend/pull/860)

### Fixed

- Return proper error when calling `POST /disbursements` with a duplicate wallet address. [#862](https://github.com/stellar/stellar-disbursement-platform-backend/pull/862)
- Properly detect memo type in receiver creation. [#870](https://github.com/stellar/stellar-disbursement-platform-backend/pull/870)
- Add Support to Direct Payments for Payments Export. [#873](https://github.com/stellar/stellar-disbursement-platform-backend/pull/873)
- Re-enable golangci-lint in the CI and address all the related issues. [#874](https://github.com/stellar/stellar-disbursement-platform-backend/pull/874)
- Fix email color overrides. [#762](https://github.com/stellar/stellar-disbursement-platform-backend/pull/762)
- Fix Receiver Invitation for direct payments. [#876](https://github.com/stellar/stellar-disbursement-platform-backend/pull/876)

## [4.0.1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/4.0.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/4.0.0...4.0.1))

### Fixed

- Disbursement remaining balance calculation fails when there are pending Direct Payments [#842](https://github.com/stellar/stellar-disbursement-platform-backend/pull/842)
- Return proper error when calling `POST /receivers` with a duplicate wallet address. [#836](https://github.com/stellar/stellar-disbursement-platform-backend/pull/836)

### Changed

- Replace Bitnami Kafka image with Apache Kafka official image due to Bitnami discontinuing support. [#844](https://github.com/stellar/stellar-disbursement-platform-backend/pull/844)
- Validate length of message template and organization name for organization patch request. [#839](https://github.com/stellar/stellar-disbursement-platform-backend/pull/839)


## [4.0.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/4.0.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.7.3...4.0.0))

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `4.0.0`.

### 🚨Potential Breaking Changes
- Added wallet address uniqueness constraint that prevents the same wallet address from being assigned to multiple receivers. [#750](https://github.com/stellar/stellar-disbursement-platform-backend/pull/750)

### Added

- Added Bridge Integration API endpoints to the backend:
  - `GET /bridge-integration` to get the current integration status.
  - `PATCH /bridge-integration` to opt into the Bridge integration or create a virtual account.
  - [#720](https://github.com/stellar/stellar-disbursement-platform-backend/pull/720)
- Added Bridge Helm chart configuration to the SDP charts [#742](https://github.com/stellar/stellar-disbursement-platform-backend/pull/742)
- Add API keys management endpoints [#677](https://github.com/stellar/stellar-disbursement-platform-backend/pull/677)
- Added a new endpoint to unregister a receiver wallet `PATCH /receivers/wallets/:id/status`. [#675](https://github.com/stellar/stellar-disbursement-platform-backend/pull/675)
- Added a new authentication middleware to support API keys. [#681](https://github.com/stellar/stellar-disbursement-platform-backend/pull/681)
- Added `POST /receivers` endpoint to create new receivers [#687](https://github.com/stellar/stellar-disbursement-platform-backend/pull/687)
- Allow the serviceaccount to be set for the ap deployment in the helm chart [#679](https://github.com/stellar/stellar-disbursement-platform-backend/pull/679)
- Add badges to the README.md file. [#691](https://github.com/stellar/stellar-disbursement-platform-backend/pull/691)
- Add endpoint for the direct payments [#693](https://github.com/stellar/stellar-disbursement-platform-backend/pull/693)
- Add `type` query parameter to `GET /payments` to filter direct/disbursement payments [#694](https://github.com/stellar/stellar-disbursement-platform-backend/pull/694)
- Update wallet POST to allow accept new asset references [#696](https://github.com/stellar/stellar-disbursement-platform-backend/pull/696)
- Update PATCH endpoint to extend update capabilities [#697](https://github.com/stellar/stellar-disbursement-platform-backend/pull/697)
- Add `supported_assets` filter to `GET /wallets` endpoint [#734](https://github.com/stellar/stellar-disbursement-platform-backend/pull/734)
- Add OTP attempt tracking and validation with maximum attempts limit and expiration checks for receiver wallet registration [#773](https://github.com/stellar/stellar-disbursement-platform-backend/pull/773)
- Add asset filtering by trustline with balance information for `GET /assets` [#791](https://github.com/stellar/stellar-disbursement-platform-backend/pull/791)
- Add AWS EKS CloudFormation deployment stacks [#519](https://github.com/stellar/stellar-disbursement-platform-backend/pull/519)

### Changed

- Update supported wallets for Pubnet and Testnet: 
  - Removed Vibrant Assist wallets from pubnet and test configuration
  - Added Vesseo and Beans App on Pubnet
  - Added XLM as a supported asset for Decaf 
  - [#793](https://github.com/stellar/stellar-disbursement-platform-backend/pull/793)
- Optimize the Dockerfile to reduce the image size. [#692](https://github.com/stellar/stellar-disbursement-platform-backend/pull/692)
- Refactor packages to reduce circular dependency issues [#816](https://github.com/stellar/stellar-disbursement-platform-backend/pull/816), [#817](https://github.com/stellar/stellar-disbursement-platform-backend/pull/817)
- Prevent unregistering user-managed wallets and wallets with in-progress payments [#683](https://github.com/stellar/stellar-disbursement-platform-backend/pull/683)

### Fixed

- Fix scheduler to only run jobs for provisioned and activated tenants, preventing execution for pending or deactivated tenants. [#795](https://github.com/stellar/stellar-disbursement-platform-backend/pull/795)
- Display clear error message during reset password when token is expired. [#783](https://github.com/stellar/stellar-disbursement-platform-backend/pull/783)
- Fix add_tests_users.sh script in main.sh for Alpine Linux. [#738](https://github.com/stellar/stellar-disbursement-platform-backend/pull/738)
- Use `sh` instead of `bash` in e2e tests for better compatibility [#736](https://github.com/stellar/stellar-disbursement-platform-backend/pull/736)

### Security and Dependencies

- Update dependencies and address security alerts [#755](https://github.com/stellar/stellar-disbursement-platform-backend/pull/755), [#756](https://github.com/stellar/stellar-disbursement-platform-backend/pull/756)
- Pin @bitnami/readme-generator-for-helm version [#731](https://github.com/stellar/stellar-disbursement-platform-backend/pull/731)
- Bump the minor-and-patch group across 1 directory with 6 updates. [#690](https://github.com/stellar/stellar-disbursement-platform-backend/pull/690)
- Bump the minor-and-patch group across 1 directory with 10 updates [#753](https://github.com/stellar/stellar-disbursement-platform-backend/pull/753), [#769](https://github.com/stellar/stellar-disbursement-platform-backend/pull/769)
- Bump docker/build-push-action in the all-actions group [#685](https://github.com/stellar/stellar-disbursement-platform-backend/pull/685), [#801](https://github.com/stellar/stellar-disbursement-platform-backend/pull/801)
- Bump the all-docker group across 1 directory with 2 updates [#709](https://github.com/stellar/stellar-disbursement-platform-backend/pull/709)
- Bump golang from 1.24.4-alpine to 1.24.6-alpine [#760](https://github.com/stellar/stellar-disbursement-platform-backend/pull/760), [#798](https://github.com/stellar/stellar-disbursement-platform-backend/pull/798)
- Bump github.com/go-chi/chi/v5 from 5.2.1 to 5.2.2 [#719](https://github.com/stellar/stellar-disbursement-platform-backend/pull/719)
- Bump github.com/go-viper/mapstructure/v2 from 2.2.1 to 2.3.0 [#752](https://github.com/stellar/stellar-disbursement-platform-backend/pull/752)
- Bump vite dependencies [#654](https://github.com/stellar/stellar-disbursement-platform-backend/pull/654)
- Update stellar/go dependency to support Protocol 23 rollout [#829](https://github.com/stellar/stellar-disbursement-platform-backend/pull/829)

## [3.7.3](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.7.3) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.7.2...3.7.3))

### Fixed
- Fix balance calculation for resumed disbursements by subtracting amount already disbursed. [#787](https://github.com/stellar/stellar-disbursement-platform-backend/pull/787)

## [3.7.2](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.7.2) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.7.1...3.7.2))

### Added
- Added Receiver's External ID to the `GET /exports/payments` endpoint. [#748](https://github.com/stellar/stellar-disbursement-platform-backend/pull/748)

### Fixed
- Fix `GET /disbursements` endpoint when `receiver_registration_message_template` is NULL. [#749](https://github.com/stellar/stellar-disbursement-platform-backend/pull/749)

## [3.7.1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.7.1) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.7.0...3.7.1))

### Fixed

- Fix stellar.expert pubnet URL configuration in helm chart. [#700](https://github.com/stellar/stellar-disbursement-platform-backend/pull/700)

## [3.7.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.7.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.6.0...3.7.0))

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.7.0`.

### Added

- Update integration tests to wait for the payment to be processed by TSS/Circle with a retry mechanism. [#585](https://github.com/stellar/stellar-disbursement-platform-backend/pull/585)
- Add Circle Payouts API to the e2e integration test. [#586](https://github.com/stellar/stellar-disbursement-platform-backend/pull/586)
- A React app for the SEP-24 interactive deposit flow. This app is served by the backend and is accessible at `/wallet-registration/start`. [#560](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/560)
- A new endpoint `GET /sep24-interactive-deposit/info` to kick off the SEP-24 interactive deposit flow. [#560](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/560)
- Ukranian, Russian and Spanish translations for the SEP-24 interactive deposit flow. [#621](https://github.com/stellar/stellar-disbursement-platform-backend/pull/621)
- Add GH Workflow to automatically set the pull request assignee. [#591](https://github.com/stellar/stellar-disbursement-platform-backend/pull/591)
- Added `priorityClassName` and `topologySpreadConstraints` properties to helm charts. [#601](https://github.com/stellar/stellar-disbursement-platform-backend/pull/601)
- Added `nodeSelector`, `tolerations` and `affinity` global defaults to helm charts. [#607](https://github.com/stellar/stellar-disbursement-platform-backend/pull/607)
- Added CMD to create default tenant `tenants ensure-default`, introduced new env variables. [#634](https://github.com/stellar/stellar-disbursement-platform-backend/pull/634)
- Added functionality and new `receiver_registration_attempts` table to track failed OTP attempts. [#635](https://github.com/stellar/stellar-disbursement-platform-backend/pull/635)
- Added automatic default tenant resolution for single tenant mode. [#616](https://github.com/stellar/stellar-disbursement-platform-backend/pull/616)

### Changed

- Allow the AWS configuration to be handled by the AWS session if the static credentials and region are not all passed to the SDP function. [#565](https://github.com/stellar/stellar-disbursement-platform-backend/pull/565)
- GET `/organization/logo` is changed to be a public (unauthenticated) endpoint. [#564](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/564)
- ReCAPTCHA is now optional for the SEP-24 interactive deposit flow. [#560](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/560)
- Simplified Helm Charts, added documentation and instructions for local set-up, auto-generation of secrets. [#596](https://github.com/stellar/stellar-disbursement-platform-backend/pull/596)
- ReCAPTCHA now allowed to be disabled in pubnet environments. [#618](https://github.com/stellar/stellar-disbursement-platform-backend/pull/618)
- Deprecated `ENABLE_SCHEDULER` and `EVENT_BROKER_TYPE=NONE` in favor of using `EVENT_BROKER_TYPE=SCHEDULER` for background jobs. [#617](https://github.com/stellar/stellar-disbursement-platform-backend/pull/617)

### Fixed

- Fix CLI tests and test assertions. [#587](https://github.com/stellar/stellar-disbursement-platform-backend/pull/587)
- Fix the error message for duplicate instructions in a disbursement. [#662](https://github.com/stellar/stellar-disbursement-platform-backend/pull/662)
- Fix `DELETE /tenants/:id` when tenant is a DB Vault tenant.[#664](https://github.com/stellar/stellar-disbursement-platform-backend/pull/664)

### Security and Dependencies

- Upgrade AWS and stellar/go dependencies. [#581](https://github.com/stellar/stellar-disbursement-platform-backend/pull/581)
- Bump github.com/golang-jwt/jwt/v4 from 4.5.1 to 4.5.2. [#588](https://github.com/stellar/stellar-disbursement-platform-backend/pull/588)
- Bump vite. [#643](https://github.com/stellar/stellar-disbursement-platform-backend/pull/643)
- Bump golang from 1.24.1-bullseye to 1.24.2-bullseye in the all-docker group. [#615](https://github.com/stellar/stellar-disbursement-platform-backend/pull/615)
- Bump golang.org/x/net from 0.37.0 to 0.38.0 in the go_modules group [#622](https://github.com/stellar/stellar-disbursement-platform-backend/pull/622)
- Bump docker/build-push-action in the all-actions group [#642](https://github.com/stellar/stellar-disbursement-platform-backend/pull/642)
- Bump the minor-and-patch group across 1 directory with 14 updates [#632](https://github.com/stellar/stellar-disbursement-platform-backend/pull/632)

## [3.6.2](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.6.2) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.6.1...3.6.2))

### Fixed

- Tenant resolution fails after tenant is deleted. [#638](https://github.com/stellar/stellar-disbursement-platform-backend/pull/638)

## [3.6.1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.6.1) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.6.0...3.6.1))

### Added

- Add support for Decaf wallet on Pubnet. [#611](https://github.com/stellar/stellar-disbursement-platform-backend/pull/611)

## [3.6.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.6.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.5.1...3.6.0))

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.6.0`.

### Added

- Add support to send payments with memos if the receiver wallet was registered with a memo, or when the flag `organizations.is_memo_tracing_enabled` is set to `true`. [#537](https://github.com/stellar/stellar-disbursement-platform-backend/pull/537)
- Add support to memos when ingesting CSV files with known wallet addresses. [#572](https://github.com/stellar/stellar-disbursement-platform-backend/pull/572)
- Add new CMD `distribution-account rotate` to rotate distribution account keys. [#579](https://github.com/stellar/stellar-disbursement-platform-backend/pull/579)

### Changed

- Reuse column names in SQL queries (DRY principle). [#535](https://github.com/stellar/stellar-disbursement-platform-backend/pull/535)
- Improve UX on the reset-password flow by embedding the reset token in the URL so it can be parsed by the FE without human intervention. [#557](https://github.com/stellar/stellar-disbursement-platform-backend/pull/557)
- Make create disbursement atomic. [#554](https://github.com/stellar/stellar-disbursement-platform-backend/pull/554)
- Refactor the PR checklist to be more user-friendly and easier to follow. [#568](https://github.com/stellar/stellar-disbursement-platform-backend/pull/568)
- Enabled scheduler instead of Kafka in the default environment variable template `.env.example`. [#573](https://github.com/stellar/stellar-disbursement-platform-backend/pull/573)
- Change `{CIRCLE_API}/ping` method to validate only the response status code and not the body. [#580](https://github.com/stellar/stellar-disbursement-platform-backend/pull/580)

### Fixed

- Preserve port numbers in SEP-24 invitation links [#567](https://github.com/stellar/stellar-disbursement-platform-backend/pull/567)
- `POST /users` fails with 500 during validation errors. [#562](https://github.com/stellar/stellar-disbursement-platform-backend/pull/562)
- Preserve port numbers in SEP-24 invitation links [#567](https://github.com/stellar/stellar-disbursement-platform-backend/pull/567)

### Security and Dependencies

- Bump golang in the all-docker group. [#552](https://github.com/stellar/stellar-disbursement-platform-backend/pull/552), [#570](https://github.com/stellar/stellar-disbursement-platform-backend/pull/570)
- Bump the all-actions group. [#569](https://github.com/stellar/stellar-disbursement-platform-backend/pull/569), [#578](https://github.com/stellar/stellar-disbursement-platform-backend/pull/578)
- Bump golang.org/x/net from 0.34.0 to 0.36.0. [#575](https://github.com/stellar/stellar-disbursement-platform-backend/pull/575)
- Bump checks versions so they work with the latest Golang versions. [#576](https://github.com/stellar/stellar-disbursement-platform-backend/pull/576)
- Bump the minor-and-patch group across 1 directory with 8 updates [#577](https://github.com/stellar/stellar-disbursement-platform-backend/pull/577)

## [3.5.1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.5.1) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.5.0...3.5.1))

### Fixed

- GET `/disbursements` breaks when one of the users is deactivated. [#550](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/550)

## [3.5.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.5.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.4.0...3.5.0))

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.5.0`.

### Added

- Added short linking for Wallet Registration Links.
  [#523](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/523)
- Added a new `is_link_shortener_enabled` property to `GET` and `PATCH` organizations endpoints to enable/disable the short link feature.
  [#523](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/523)
- Added receiver contact info for Payments export.
  [#538](https://github.com/stellar/stellar-disbursement-platform-frontend/pull/538)

## [3.4.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.4.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.3.0...3.4.0))

Release of the Stellar Disbursement Platform `v3.4.0`. This release adds support for `q={term}` query searches in the
`GET /payments` endpoint, and updates the CSV parser to ignore BOM (Byte Order Mark) characters.

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.4.0`.

### Changed

- Update the `GET /payments` endpoint to accept `q={term}` query searches. [#530](https://github.com/stellar/stellar-disbursement-platform-backend/pull/530)
- Update the CSV parser to ignore BOM (Byte Order Mark) characters. [#531](https://github.com/stellar/stellar-disbursement-platform-backend/pull/531)

### Security and Dependencies

- Bump golang in the all-docker group. [#507](https://github.com/stellar/stellar-disbursement-platform-backend/pull/507)
- Bump the all-actions group. [#514](https://github.com/stellar/stellar-disbursement-platform-backend/pull/514)
- Bump the minor-and-patch group. [#529](https://github.com/stellar/stellar-disbursement-platform-backend/pull/529)

## [3.3.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.3.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.2.0...3.3.0))

Release of the Stellar Disbursement Platform `v3.3.0`. This release adds support to Circle's Transfers API, as an
alternative to the Payouts API. It also adds audit functionality for the `receivers` table to track changes.

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.3.0`.

### Added

- Audit functionality for the `receivers` table to track changes. [#520](https://github.com/stellar/stellar-disbursement-platform-backend/pull/520)
- Support for Circle API type `TRANSFERS`, and allow users to choose between `TRANSFERS` and `PAYOUTS` through the `CIRCLE_API_TYPE` environment variable. It defaults to `TRANSFERS`. [#522](https://github.com/stellar/stellar-disbursement-platform-backend/pull/522)

### Changed

- Refactor MFA and reCAPTCHA handling for better modularity in preparation for API-only usage. [#499](https://github.com/stellar/stellar-disbursement-platform-backend/pull/499)

### Removed

- Removed `EC256_PUBLIC_KEY` environment variable as it can be derived from the `EC256_PRIVATE_KEY` [#511](https://github.com/stellar/stellar-disbursement-platform-backend/pull/511)
- Removed `nginx.ingress.kubernetes.io/server-snippet` annotation from SDP and AP ingress resources. [#510](https://github.com/stellar/stellar-disbursement-platform-backend/pull/510)

## [3.2.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.2.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.1.0...3.2.0))

Release of the Stellar Disbursement Platform `v3.2.0`. This release focuses on enhancing the platform’s reliability and
data tracking capabilities. Users can now patch already confirmed verification fields for receivers, providing greater
flexibility in managing locked-out accounts. Additionally, audit logging has been introduced to track changes made to
critical verification data, ensuring improved accountability and transparency.

> [!WARNING]
> This version is compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.2.0`.

### Added

- Dynamic Audit Table Creation through the `create_audit_table` Postgres function. This is initially applied to the receiver_verifications table to track changes. [#513](https://github.com/stellar/stellar-disbursement-platform-backend/pull/513)

### Changed

- Enabled patching of already confirmed verification fields for receivers, addressing scenarios where users might get locked out of a partner’s system. [#512](https://github.com/stellar/stellar-disbursement-platform-backend/pull/512)

## [3.1.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.1.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/3.0.0...3.1.0))

Release of the Stellar Disbursement Platform `v3.1.0`. This release introduces key updates, including the migration to
Circle's Payouts API, aligning with Circle's latest recommendations. It also enhances platform functionality by enabling
data export through dedicated endpoints, allowing users to export disbursements, payments, and receivers with filters.
Additionally, users now have the ability to delete disbursements in `DRAFT` or `READY` status, streamlining data
management workflows.

> [!WARNING]
> This version is only compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.1.0`.

### Added

- Export functionality, allowing users to export:
  - Disbursements with filters. [#490](https://github.com/stellar/stellar-disbursement-platform-backend/pull/490)
  - Payments with filters. [#493](https://github.com/stellar/stellar-disbursement-platform-backend/pull/493)
  - Receivers with filters. [#496](https://github.com/stellar/stellar-disbursement-platform-backend/pull/496)
- Option to delete a disbursement in `DRAFT` or `READY` status. [#487](https://github.com/stellar/stellar-disbursement-platform-backend/pull/487)
- Add futurenet as one of the e2e tests scenarios applied to the e2e GitHub Action. [#472](https://github.com/stellar/stellar-disbursement-platform-backend/pull/472)

### Changed

- Update Circle API to use Circle payouts, which is the new officially suggested (and supported) API. [#486](https://github.com/stellar/stellar-disbursement-platform-backend/pull/486), [#491](https://github.com/stellar/stellar-disbursement-platform-backend/pull/491)
- Only execute the GitHub e2e tests workflow prior to publishing Docker images, removing it from the pull requests test suite. [#479](https://github.com/stellar/stellar-disbursement-platform-backend/pull/479)
- Simplify docker compose by making Kafka optional and defaulting to scheduled jobs. [#481](https://github.com/stellar/stellar-disbursement-platform-backend/pull/481)
- Make Dashboard User E-mails case insensitive. [#485](https://github.com/stellar/stellar-disbursement-platform-backend/pull/485)

### Fixed

- Fix XLM support on the integration tests. [#470](https://github.com/stellar/stellar-disbursement-platform-backend/pull/470)
- Fix `main.sh` script so that it doesn't kill non-sdp containers. [#480](https://github.com/stellar/stellar-disbursement-platform-backend/pull/480)
- Skip patching transaction in AP for known-wallet address payments. [#482](https://github.com/stellar/stellar-disbursement-platform-backend/pull/482)
- Workaround for Circle's bug where retries are not handled correctly when a trustline is missing. [#504](https://github.com/stellar/stellar-disbursement-platform-backend/pull/504)
- Fix default tenant resolution during SEP24 registration. [#505](https://github.com/stellar/stellar-disbursement-platform-backend/pull/505)

### Security and Dependencies

- Prevent any html (encoded or not) in the invite templates set by staff users. [494](https://github.com/stellar/stellar-disbursement-platform-backend/pull/494)
- Bump dependencies:
  - github.com/stretchr/testify from 1.9.0 to 1.10.0. [#471](https://github.com/stellar/stellar-disbursement-platform-backend/pull/471)
  - github.com/nyaruka/phonenumbers from 1.4.2 to 1.4.3. [#483](https://github.com/stellar/stellar-disbursement-platform-backend/pull/483)
  - minor-and-patch group with 3 updates. [#489](https://github.com/stellar/stellar-disbursement-platform-backend/pull/489)
  - golang.org/x/crypto from 0.30.0 to 0.31.0. [#492](https://github.com/stellar/stellar-disbursement-platform-backend/pull/492)
  - minor-and-patch group across 1 directory with 5 updates. [#498](https://github.com/stellar/stellar-disbursement-platform-backend/pull/498)
  - github.com/twilio/twilio-go from 1.23.8 to 1.23.9. [#500](https://github.com/stellar/stellar-disbursement-platform-backend/pull/500)
- Bump docker/build-push-action from 6.9.0 to 6.11.0 in the all-actions group. [#484](https://github.com/stellar/stellar-disbursement-platform-backend/pull/484), [#501](https://github.com/stellar/stellar-disbursement-platform-backend/pull/501)
- Bump golang from 1.23.3-bullseye to 1.23.4-bullseye in the all-docker group. [#488](https://github.com/stellar/stellar-disbursement-platform-backend/pull/488)

## [3.0.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/3.0.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/2.1.1...3.0.0))

Release of the Stellar Disbursement Platform `v3.0.0`. In this release, receiver registration does not need to be done
exclusively through SMS as it now supports new types. The options are `PHONE_NUMBER`, `EMAIL`,
`EMAIL_AND_WALLET_ADDRESS`, and `PHONE_NUMBER_AND_WALLET_ADDRESS`. If a receiver is registered with a wallet address,
they can receive the payment right away without having to go through the SEP-24 registration flow.

> [!WARNING]
> This version is only compatible with the [stellar/stellar-disbursement-platform-frontend] version `3.0.0`.

### Breaking Changes

- Renamed properties and environment variables related to Email Registration Support [#412](https://github.com/stellar/stellar-disbursement-platform-backend/pull/412)
  - Renamed `MAX_INVITATION_SMS_RESEND_ATTEMPT` environment variable to `MAX_INVITATION_RESEND_ATTEMPTS`
  - Renamed `organization.sms_resend_interval` to `organization.receiver_invitation_resend_interval_days`
  - Renamed `organization.sms_registration_message_template` to `organization.receiver_registration_message_template`
  - Renamed `disbursement.sms_registration_message_template` to `disbursement.receiver_registration_message_template`

### Added

- Ability to register receivers using email addresses
  - Update the `receiver_registered_successfully.tmpl` HTML template to display the contact info [#418](https://github.com/stellar/stellar-disbursement-platform-backend/pull/418)
  - Update `/wallet-registration/verification` to accommodate different verification methods [#416](https://github.com/stellar/stellar-disbursement-platform-backend/pull/416)
  - Update send and auto-retry invitation scheduler job to work with email [#415](https://github.com/stellar/stellar-disbursement-platform-backend/pull/415)
  - Update `POST /wallet-registration/otp` to send OTPs through email [#413](https://github.com/stellar/stellar-disbursement-platform-backend/pull/413)
  - Rename SMS-related fields in `organization` and `disbursement` to be more generic [#412](https://github.com/stellar/stellar-disbursement-platform-backend/pull/412)
  - Update process disbursement instructions to accept email addresses [#404](https://github.com/stellar/stellar-disbursement-platform-backend/pull/404)
  - Add an initial screen so receivers can choose between phone number and email registration during registration [#406](https://github.com/stellar/stellar-disbursement-platform-backend/pull/406)
  - Add message channel priority to the `organizations` table [#400](https://github.com/stellar/stellar-disbursement-platform-backend/pull/400)
  - Add `MessageDispatcher` to SDP to send messages to different channels [#391](https://github.com/stellar/stellar-disbursement-platform-backend/pull/391)
  - Update the development endpoint `DELETE .../phone-number/...` to `DELETE .../contact-info/...`, allowing it to delete based on the email as well [#438](https://github.com/stellar/stellar-disbursement-platform-backend/pull/438)
  - Remove the word "phone" from the default organization's `otp_message_template` [#439](https://github.com/stellar/stellar-disbursement-platform-backend/pull/439)
  - Rename SMS-related field and update Helm docs [#468](https://github.com/stellar/stellar-disbursement-platform-backend/pull/468)
- Ability to register receivers with a Stellar wallet address directly by providing contact info and a wallet address. The options currently are `PHONE_NUMBER_AND_WALLET_ADDRESS` and `EMAIL_AND_WALLET_ADDRESS`
  - Create `GET /registration-contact-types` endpoint [#451](https://github.com/stellar/stellar-disbursement-platform-backend/pull/451)
  - Update `POST /disbursements` and `GET /disbursements` APIs to persist and return the Registration Contact Type [#452](https://github.com/stellar/stellar-disbursement-platform-backend/pull/452), [#454](https://github.com/stellar/stellar-disbursement-platform-backend/pull/454)
  - Allow `disbursement.verification_field` to be empty [#456](https://github.com/stellar/stellar-disbursement-platform-backend/pull/456)
  - Integrate wallet address in processing disbursement instructions [#453](https://github.com/stellar/stellar-disbursement-platform-backend/pull/453)
  - Add user-managed wallets [#458](https://github.com/stellar/stellar-disbursement-platform-backend/pull/458)
- Add Twilio SendGrid as a supported email client [#444](https://github.com/stellar/stellar-disbursement-platform-backend/pull/444)

### Changed

- Replaced deprecated Circle Accounts API by adopting the Circle API endpoints `GET /v1/businessAccount/balances` and `GET /configuration` [#433](https://github.com/stellar/stellar-disbursement-platform-backend/pull/433)
- `PATCH /receiver` now allows patching the phone number and email address of a receiver [#436](https://github.com/stellar/stellar-disbursement-platform-backend/pull/436)
- Increased window for clients to perform token refresh [#437](https://github.com/stellar/stellar-disbursement-platform-backend/pull/437)
- Other technical changes ([#383](https://github.com/stellar/stellar-disbursement-platform-backend/pull/383), [#450](https://github.com/stellar/stellar-disbursement-platform-backend/pull/450))

### Fixed

- Unable to get a token from the Forgot Password flow after messaging service failure [#466](https://github.com/stellar/stellar-disbursement-platform-backend/pull/466)
- ReCaptcha blocks retrying verification during wallet registration [#473](https://github.com/stellar/stellar-disbursement-platform-backend/pull/473)

### Removed

- Removed countries from the flow and deleted any references to them from the database [#455](https://github.com/stellar/stellar-disbursement-platform-backend/pull/455), [#462](https://github.com/stellar/stellar-disbursement-platform-backend/pull/462)

### Security and Dependencies

- Fix HTML injection vulnerability [#419](https://github.com/stellar/stellar-disbursement-platform-backend/pull/419)
- Fix HTML escaping [#420](https://github.com/stellar/stellar-disbursement-platform-backend/pull/420)
- Removed support for the HTTP headers `X-XSS-Protection`, `X-Forwarded-Host`, `X-Real-IP`, and `True-Client-IP` [#448](https://github.com/stellar/stellar-disbursement-platform-backend/pull/448)
- Improved validation to ensure the instruction file being uploaded is a `*.csv` file [#443](https://github.com/stellar/stellar-disbursement-platform-backend/pull/443)
- Ensure validation of URLs with the HTTPS schema on Pubnet [#445](https://github.com/stellar/stellar-disbursement-platform-backend/pull/445)
- Add path validation to the `readDisbursementCSV` method used in integration tests [#437](https://github.com/stellar/stellar-disbursement-platform-backend/pull/437)
- Bump `golangci/golangci-lint-action` [#380](https://github.com/stellar/stellar-disbursement-platform-backend/pull/380)
- Bump `golang` in the all-docker group [#387](https://github.com/stellar/stellar-disbursement-platform-backend/pull/387), [#394](https://github.com/stellar/stellar-disbursement-platform-backend/pull/394), [#414](https://github.com/stellar/stellar-disbursement-platform-backend/pull/414)
- Bump minor and patch dependencies across directories [#381](https://github.com/stellar/stellar-disbursement-platform-backend/pull/381), [#395](https://github.com/stellar/stellar-disbursement-platform-backend/pull/395), [#403](https://github.com/stellar/stellar-disbursement-platform-backend/pull/403), [#411](https://github.com/stellar/stellar-disbursement-platform-backend/pull/411), [#429](https://github.com/stellar/stellar-disbursement-platform-backend/pull/429), [#430](https://github.com/stellar/stellar-disbursement-platform-backend/pull/430), [#431](https://github.com/stellar/stellar-disbursement-platform-backend/pull/431), [#441](https://github.com/stellar/stellar-disbursement-platform-backend/pull/441).

## [2.1.1](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/2.1.1) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/2.1.0...2.1.1))

### Changed

- Removed calls related to the deprecated Circle Accounts API and replaced them with calls to `GET /v1/businessAccount/balances` and `GET /configuration`.  [#433](https://github.com/stellar/stellar-disbursement-platform-backend/pull/433).

## [2.1.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/2.1.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/2.0.0...2.1.0))

Release of the Stellar Disbursement Platform v2.1.0. This release introduces
the option to set different distribution account signers per tenant, as well
as Circle support, so the tenant can choose to run their payments through the
Circle API rather than directly on the Stellar network.

> [!WARNING]
> This version is only compatible with the [stellar/stellar-disbursement-platform-frontend] version `2.1.0`.

### Changed

- Update the name of the account types used for Distribution Accounts to be more descriptive. [#285](https://github.com/stellar/stellar-disbursement-platform-backend/pull/285), [#311](https://github.com/stellar/stellar-disbursement-platform-backend/pull/311)
- When provisioning a tenant, indicate the Distribution account signer type [#319](https://github.com/stellar/stellar-disbursement-platform-backend/pull/319)
- The DistributionAccountResolver now surfaces the tenant's CircleWalletID for Circle-using tenants [#328](https://github.com/stellar/stellar-disbursement-platform-backend/pull/328)
- Disable asset management calls when the tenant is using Circle [#322](https://github.com/stellar/stellar-disbursement-platform-backend/pull/322)
- Bump version of [github.com/stellar/go](https://github.com/stellar/go) to become compatible with Protocol 21.

### Added

- Add a new verification type called `YEAR_MONTH` [#369](https://github.com/stellar/stellar-disbursement-platform-backend/pull/369)
- Add the ability to use different signature types per tenant, allowing for more flexibility in the signature service. [#289](https://github.com/stellar/stellar-disbursement-platform-backend/pull/289)
- Add support to provision tenants with `accountType=DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT` [#330](https://github.com/stellar/stellar-disbursement-platform-backend/pull/330)
- Circle SDK integration for the backend. [#321](https://github.com/stellar/stellar-disbursement-platform-backend/pull/321)
- Implement CircleService on top of the CircleClient, in order to automatically route the calls through the correct tenant based on the tenant value saved in the context [#331](https://github.com/stellar/stellar-disbursement-platform-backend/pull/331)
- Add support for Circle-using tenants when validating the tenant available balance upon disbursement start [#309](https://github.com/stellar/stellar-disbursement-platform-backend/pull/309), [#336](https://github.com/stellar/stellar-disbursement-platform-backend/pull/336)
- Implement [joho/godotenv](https://github.com/joho/godotenv) loader [#324](https://github.com/stellar/stellar-disbursement-platform-backend/pull/324)
- Add support for Circle-using tenants to the `db setup-for-network` CLI command [#327](https://github.com/stellar/stellar-disbursement-platform-backend/pull/327)
- Implement the `GET /balances` endpoint that returns the Circle balance when the tenant is using Circle [#325](https://github.com/stellar/stellar-disbursement-platform-backend/pull/325), [#329](https://github.com/stellar/stellar-disbursement-platform-backend/pull/329)
- Implement the `PATCH /organization/circle-config` endpoint that allows Circle configuration to be updated for Circle-using tenants [#326](https://github.com/stellar/stellar-disbursement-platform-backend/pull/326), [#332](https://github.com/stellar/stellar-disbursement-platform-backend/pull/332), [#334](https://github.com/stellar/stellar-disbursement-platform-backend/pull/334)
- Send Stellar payments through Circle when the tenant uses a CIRCLE distribution account [#333](https://github.com/stellar/stellar-disbursement-platform-backend/pull/333)
- Implement Circle reconciliation through polling [#339](https://github.com/stellar/stellar-disbursement-platform-backend/pull/339), [#347](https://github.com/stellar/stellar-disbursement-platform-backend/pull/347)
- Add Circle transfer ID to GET /payments endpoints [#346](https://github.com/stellar/stellar-disbursement-platform-backend/pull/346)
- Add function to migrate data from a single-tenant to a multi-tenant instance [#351](https://github.com/stellar/stellar-disbursement-platform-backend/pull/351)
- Invalidate Circle Distribution Account Status upon receiving auth error [#350](https://github.com/stellar/stellar-disbursement-platform-backend/pull/350), [359](https://github.com/stellar/stellar-disbursement-platform-backend/pull/359)
- Add retry for circle 429 requests [#362](https://github.com/stellar/stellar-disbursement-platform-backend/pull/362)
- Separate Stellar and Circle payment jobs [#366](https://github.com/stellar/stellar-disbursement-platform-backend/pull/366), [#374](https://github.com/stellar/stellar-disbursement-platform-backend/pull/374)
- Misc
  - Reformat the imports using goimports and enforce it through a GH Action [#337](https://github.com/stellar/stellar-disbursement-platform-backend/pull/337)
  - Added dependabot extra features [#349](https://github.com/stellar/stellar-disbursement-platform-backend/pull/349)
  - Add CI for e2e integration test for Circle [#342](https://github.com/stellar/stellar-disbursement-platform-backend/pull/342), [#357](https://github.com/stellar/stellar-disbursement-platform-backend/pull/357)
  - Add CI to validate single-tenant to multi-tenant migration [#356](https://github.com/stellar/stellar-disbursement-platform-backend/pull/356)

### Fixed

- Fix SDP helm charts [#323](https://github.com/stellar/stellar-disbursement-platform-backend/pull/323), [#375](https://github.com/stellar/stellar-disbursement-platform-backend/pull/375)
- Fix CF 429 response and anchor patch transaction job for circle accounts [#361](https://github.com/stellar/stellar-disbursement-platform-backend/pull/361)
- Select the correct error object used in a crash-reporter alert [#365](https://github.com/stellar/stellar-disbursement-platform-backend/pull/365)
- Fixes post python migration [#367](https://github.com/stellar/stellar-disbursement-platform-backend/pull/367)
- Make `PATCH /receivers/:id` validation consistent [#371](https://github.com/stellar/stellar-disbursement-platform-backend/pull/371)

### Security

- Security patch for gorilla/schema and rs/cors [#345](https://github.com/stellar/stellar-disbursement-platform-backend/pull/345)
- Bump versions of getsentry/sentry-go and gofiber/fiber [#352](https://github.com/stellar/stellar-disbursement-platform-backend/pull/352)

### Deprecated

- Deprecated the use of `DISTRIBUTION_SIGNER_TYPE`, since this information is now provided when provisioning a tenant [#319](https://github.com/stellar/stellar-disbursement-platform-backend/pull/319).
- Remove Freedom Wallet from the list of pubnet wallets [#372](https://github.com/stellar/stellar-disbursement-platform-backend/pull/372)

## [2.0.0](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/2.0.0) ([diff](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.7...2.0.0))

Release of the Stellar Disbursement Platform v2.0.0. This
release introduces multi-tenancy support, allowing multiple tenants
(organizations) to use the platform simultaneously.

Each organization has its own set of users, receivers, disbursements, etc.

> [!WARNING]
> This version is only compatible with the [stellar/stellar-disbursement-platform-frontend] version `2.0.0`.

### Changed

- Support multi-tenant CLI
  - Make `add-user` CLI support multi-tenancy [#228](https://github.com/stellar/stellar-disbursement-platform-backend/pull/228)
  - Change migrations CLI to run for all tenants [#89](https://github.com/stellar/stellar-disbursement-platform-backend/pull/89)
- Use DB connection pool as dependency injection [#207](https://github.com/stellar/stellar-disbursement-platform-backend/pull/207)
- Make receiver registration handler tenant-aware [#117](https://github.com/stellar/stellar-disbursement-platform-backend/pull/117)
- Tag log entries with tenant metadata [#192](https://github.com/stellar/stellar-disbursement-platform-backend/pull/192)
- Use `DistributionAccountResolver` instead of passing around distribution public key [#212](https://github.com/stellar/stellar-disbursement-platform-backend/pull/212)
- Make provision new tenant an atomic operation [#233](https://github.com/stellar/stellar-disbursement-platform-backend/pull/233)
- Make `ready_payments_cancellation` job multi-tenant [#223](https://github.com/stellar/stellar-disbursement-platform-backend/pull/223)

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
  - `DELETE` Tenant API [#272](https://github.com/stellar/stellar-disbursement-platform-backend/pull/272)
- Patch incoming TSS events to Anchor platform [#134](https://github.com/stellar/stellar-disbursement-platform-backend/pull/134)
- Update DB structure so that TSS resources can be shared by multiple SDP tenants
  - Move all TSS related tables to TSS schema [#141](https://github.com/stellar/stellar-disbursement-platform-backend/pull/141)
  - Create TSS schema and migrations CLI command [#136](https://github.com/stellar/stellar-disbursement-platform-backend/pull/136)
  - Refactor migrations commands to support TSS migrations [#123](https://github.com/stellar/stellar-disbursement-platform-backend/pull/123)
- Add host distribution account awareness [#172](https://github.com/stellar/stellar-disbursement-platform-backend/pull/172)
- Wire distribution account to tenant admin table during user provisioning [#198](https://github.com/stellar/stellar-disbursement-platform-backend/pull/198)
- Prepare transaction submission table to reference tenant [#142](https://github.com/stellar/stellar-disbursement-platform-backend/pull/142)
- Kafka message broker support
  - Migrate SMS invitation to use message broker from scheduled jobs [#133](https://github.com/stellar/stellar-disbursement-platform-backend/pull/133)
  - Publish receiver wallet invitation events at disbursement start [#182](https://github.com/stellar/stellar-disbursement-platform-backend/pull/182)
  - Produce payment events to sync back to SDP [#149](https://github.com/stellar/stellar-disbursement-platform-backend/pull/149)
  - Produce payment events from SDP to TSS [#159](https://github.com/stellar/stellar-disbursement-platform-backend/pull/159)
- Implement `DistributionAccountDBSignatureClient` [#197](https://github.com/stellar/stellar-disbursement-platform-backend/pull/197)
- Create tenant distribution account during provisioning [#224](https://github.com/stellar/stellar-disbursement-platform-backend/pull/224)
- Enable payments scheduler job as an alternative to using Kafka [#230](https://github.com/stellar/stellar-disbursement-platform-backend/pull/230)
- Add default tenant capability to start the SDP in a single tenant mode [#249](https://github.com/stellar/stellar-disbursement-platform-backend/pull/249)
- Add script to migrate SDP v1.1.6 to V2.x.x [#267](https://github.com/stellar/stellar-disbursement-platform-backend/pull/267)

### Security

- Admin API authentication/authorization [#201](https://github.com/stellar/stellar-disbursement-platform-backend/pull/201)
- Enable security protocols for Kafka
  - SASL auth [#162](https://github.com/stellar/stellar-disbursement-platform-backend/pull/162)
  - SSL auth [#226](https://github.com/stellar/stellar-disbursement-platform-backend/pull/226)

## [1.1.7](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.6...1.1.7)

### Security

- Bump google.golang.org/protobuf from 1.31.0 to 1.33.0. [#270](https://github.com/stellar/stellar-disbursement-platform-backend/pull/270)
- Bump golang.org/x/crypto from v0.17.0 to v0.21.0. [#269](https://github.com/stellar/stellar-disbursement-platform-backend/pull/269)

## [1.1.6](https://github.com/stellar/stellar-disbursement-platform-backend/compare/1.1.5...1.1.6)

Attention, this version is compatible with the frontend version [1.1.2](https://github.com/stellar/stellar-disbursement-platform-frontend/releases/tag/1.1.2).

### Changed

- Update the `PATCH /receivers/{id}` request, so a receiver's verification info is not just inserted but upserted. The update part of the upsert only takes place if the verification info has not been confirmed yet. [#205](https://github.com/stellar/stellar-disbursement-platform-backend/pull/205)
- Update the order of the verification field that is shown to the receiver during the [SEP-24] flow. The order was `(updated_at DESC)` and was updated to the composed sorting `(updated_at DESC, rv.verification_field ASC)` to ensure consistency when multiple verification fields share the same `updated_at` value.
- Improve information in the error message returned when the disbursement instruction contains a verification info that is different from an already existing verification info that was already confirmed by the receiver. [#178](https://github.com/stellar/stellar-disbursement-platform-backend/pull/178)
- When adding an asset, make sure to trim the spaces fom the issuer field. [#185](https://github.com/stellar/stellar-disbursement-platform-backend/pull/185)

### Security

- Bump Go version from 1.19 to 1.22, and upgraded the version of some CI tools. [#196](https://github.com/stellar/stellar-disbursement-platform-backend/pull/196)
- Add rate-limiter in both in the application and the kubernetes deployment. [#195](https://github.com/stellar/stellar-disbursement-platform-backend/pull/195)

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

- [SEP-24] registration flow not working properly when the phone number was not found in the DB [#187](https://github.com/stellar/stellar-disbursement-platform-backend/pull/187)
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
- Change [SEP-24] Flow to display different verifications based on disbursement verification type [#116](https://github.com/stellar/stellar-disbursement-platform-backend/pull/116)
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
- [SEP-10] domain configuration for Vibrant wallet on Testnet. [#42](https://github.com/stellar/stellar-disbursement-platform-backend/pull/42)
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
their phone number and additional customer data through the [SEP-24] interactive deposit flow, where this data is shared
directly with the backend through a webpage inside the wallet, but the wallet itself does not have access to this data.

Upon successful verification, the SDP will transfer the funds directly to the recipient's wallet. When the recipient's
wallet has been successfully associated with their phone number in the SDP, all subsequent payments will occur
automatically.

[stellar/stellar-disbursement-platform-frontend]: https://github.com/stellar/stellar-disbursement-platform-frontend
[SEP-10]: https://stellar.org/protocol/sep-10
[SEP-24]: https://stellar.org/protocol/sep-24
