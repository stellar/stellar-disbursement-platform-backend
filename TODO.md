# TODOs

As part of this PR, don't forget to:

- [x] NewDefaultSignatureServiceNew
- [x] Tests for (opts *DefaultSignatureServiceOptions) Validate() error
- [ ] Update channel-account view method signature. No options is needed
- [ ] Update create and update to use the flag again, so we don't break the code
- [ ] Add tests for dependency injectors
  - [ ] tss_db_connection_pool
  - [ ] signature_service
- [ ] revert `channel-account-encryption-passphrase`
- [ ] Add documentation saying to use `go run main.go channel-accounts delete --delete-all-accounts --channel-account-encryption-key=$DISTRIBUTION_SEED` to delete all previous accounts, and then recreate them with ``go run main.go channel-accounts ensure --num-channel-accounts-ensure={whatever}`
