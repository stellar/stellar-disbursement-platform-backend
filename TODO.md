# TODOs

As part of this PR, don't forget to:

- [x] NewDefaultSignatureServiceNew
- [x] Tests for (opts *DefaultSignatureServiceOptions) Validate() error
- [x] Update channel-account view method signature. No options object is needed
- [x] create/ensure commands now don't use flags anymore. Handle it in the helm chart
- [x] handle helmchart and stellar/kube around the variable `channel-account-encryption-passphrase`
- [⏳] Add tests for dependency injectors
  - [ ] tss_db_connection_pool
  - [ ] signature_service
  - [x] New methods in channel_accounts_service.go
- [ ] Add documentation saying to use `go run main.go channel-accounts delete --delete-all-accounts --channel-account-encryption-key=$DISTRIBUTION_SEED` to delete all previous accounts, and then recreate them with ``go run main.go channel-accounts ensure --num-channel-accounts-ensure={whatever}`
