Release `{{version}}` to `dev`

### Pending Tasks

- [ ] **Merge the main PR first**: {{ main_pr_url }}
- [ ] **Rebase this branch** onto `main` after main PR is merged
- [ ] **Review CI checks** - Ensure all checks pass

### Contract Changes Detection

Contract change detection ran automatically during release preparation.

**Status**: {{contracts_changed}}

If contracts were modified:
- [ ] Run the `Contract WASM Artifacts` workflow on `develop`
- [ ] Attach generated WASM files to the GitHub release draft
- [ ] Update release notes to mention contract changes

### Merge Instructions 🚨

- [ ] Merge this PR using the **`Merge pull request`** button (do NOT squash or rebase)
