# TODOs

As part of this PR, don't forget to:

- [ ] Update deployments and docs with:
  - [ ] `channel-account-encryption-passphrase`
  - [ ] Add documentation saying to use `go run main.go channel-accounts delete --delete-all-accounts --channel-account-encryption-key=$DISTRIBUTION_SEED` to delete all previous accounts, and then recreate them with ``go run main.go channel-accounts ensure --num-channel-accounts-ensure={whatever}`
