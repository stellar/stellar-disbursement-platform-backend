Release `{{version}}` to `main`

### Automated Preparation ✅

The following tasks have been completed automatically by the release workflow:

- [x] Bump version in main.go
- [x] Bump version in helmchart/sdp/Chart.yaml (version and appVersion)
- [x] Bump backend image tag in helmchart/sdp/values.yaml
- [x] Bump frontend image tag in helmchart/sdp/values.yaml
- [x] Bump backend and frontend image tags in cloudformation/eks/helm/values-mainnet.yaml
- [x] Bump backend and frontend image tags in cloudformation/eks/helm/values-testnet.yaml
- [x] Update CHANGELOG.md release entry (Unreleased + optional AI gap-fill)
- [x] Regenerate helmchart/sdp/README.md

### Manual Review Required 👀

Please review the automated changes before merging:

- [ ] **CHANGELOG.md** - Verify release notes are accurate and include missing merged PRs
- [ ] **Version consistency** - Verify all version bumps are correct across files
- [ ] **Helm README** - Ensure it's in sync with values.yaml

### Merge Instructions 🚨

- [ ] Merge this PR using the **`Merge pull request`** button (do NOT squash or rebase)
