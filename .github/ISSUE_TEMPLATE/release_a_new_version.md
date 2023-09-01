---
name: Release a New Version!
about: Prepare a release to be launched
title: ''
labels: release
---

<!-- Please Follow this checklist before making your release. Thanks! -->

## Release Checklist

> Attention: the examples below use the version `x.y.z` but you should update them to use the version you're releasing.

### Git Preparation

- [ ] Decide on a version number based on the current version number and the common rules defined in [Semantic Versioning](https://semver.org). E.g. `x.y.z`.
- [ ] Update this ticket name to reflect the new version number, following the pattern "Release `x.y.z`".
- [ ] Cut a branch for the new release out of the `develop` branch, following the gitflow naming pattern `release/x.y.z`.

### Code Preparation

- [ ] Update the code to use this version number.
  - [ ] Update `version` and `appVersion` in [helmchart/sdp/Chart.yaml].
  - [ ] Update the constant `Version` in [main.go]
- [ ] Update the [CHANGELOG.md] file with the new version number and release notes.
- [ ] Run tests and linting, and make sure the version running in the default branch is working end-to-end. At least the minimal end-to-end manual tests is mandatory.
- [ ] 🚨 DO NOT RELEASE before holidays or weekends! Mondays and Tuesdays are preferred.

### Merging the Branches

- [ ] When the team is confident the release is stable, you'll need to create two pull requests:
  - [ ] `release/x.y.z -> main`: 🚨 Do not squash-and-merge! This PR should be merged with a merge commit.
  - [ ] `release/x.y.z -> develop`: this should be merged after the `main` branch is merged. 🚨 Do not squash-and-merge! This PR should be merged with a merge commit.

### Publishing the Release

- [ ] After the release branch is merged to `main`, create a new release on GitHub with the name `x.y.z` and the use the same changes from the [CHANGELOG.md] file.
  - [ ] The release should automatically publish a new version of the docker image to Docker Hub. Double check if that happened.
- [ ] Propagate the helmchart version update to the https://github.com/stellar/helm-charts repository.

[main.go]: https://github.com/stellar/stellar-disbursement-platform-backend/blob/develop/main.go
[helmchart/sdp/Chart.yaml]: https://github.com/stellar/stellar-disbursement-platform-backend/blob/develop/helmchart/sdp/Chart.yaml
[CHANGELOG.md]: https://github.com/stellar/stellar-disbursement-platform-backend/blob/develop/CHANGELOG.md
