Release `{{version}}`

## Release Checklist

### Git Preparation

- [x] Create release branch `release/{{version}}` from `develop`
- [x] Create pull requests:
  - Main PR: {{ main_pr_url }}
  - Dev PR: {{ dev_pr_url }}

### Code Preparation

- [ ] Run tests and linting
- [ ] Complete the checklist and merge the main PR: {{ main_pr_url }}
- [ ] Complete the checklist and merge the dev PR: {{ dev_pr_url }}
- [ ] 🚨 DO NOT RELEASE before holidays or weekends! Mondays and Tuesdays are preferred.

### Publishing the Release

- [ ] After the main PR is merged, publish the draft release: {{ release_url }} -> [Release Page](https://github.com/stellar/stellar-disbursement-platform-backend/releases/tag/{{version}})
  - [ ] Verify the Docker image is published to [Docker Hub](https://hub.docker.com/r/stellar/stellar-disbursement-platform-backend/tags)
- [ ] Propagate the helmchart version update to the [stellar/helm-charts](https://github.com/stellar/helm-charts) repository
