### What

[TODO: Short statement about what is changing.]

### Why

[TODO: Why this change is being made. Include any context required to understand the why.]

### Known limitations

[TODO or N/A]

### Checklist

#### PR Structure

* [ ] This PR has a reasonably narrow scope (if not, break it down into smaller PRs).
* [ ] This PR does not mix refactoring changes with feature changes (split into two PRs otherwise).
* [ ] This PR adds tests for the new functionality or fixes.

#### PR Description

* [ ] This PR title and description are clear enough for anyone to review it.
* [ ] This PR title starts with the Jira ticket code, or the subject of the PR (e.g. `SDP-1234: Add new feature` or `Chore: Refactor package xyz`).

#### Configs and Secrets

* [ ] No new CONFIG variables are required -OR- the new required ones were added to the helmchart and deployments.
* [ ] No new SECRETS variables are required -OR- the new required ones were mentioned in the helmchart and added to the deployments.

#### Release

* [ ] This PR updates the `CHANGELOG.md` file.
* [ ] This is not a breaking change.
* [ ] The PR preview is working as expected.
* [ ] **This is ready for production.**. If your PR is not ready for production, please consider opening additional complementary PRs using this one as the base. Only merge this into `develop` or `main` after it's ready for production!
