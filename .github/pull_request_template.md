### What

[TODO: Short statement about what is changing.]

### Why

[TODO: Why this change is being made. Include any context required to understand the why.]

### Known limitations

[TODO or N/A]

### Checklist

#### PR Structure

* [ ] This PR has reasonably narrow scope (if not, break it down into smaller PRs).
* [ ] This PR does not mix refactoring changes with feature changes (split into two PRs otherwise).
* [ ] This PR's title starts with the name of the package, area, or subject affected by the change.

#### Thoroughness

* [ ] This PR adds tests for the new functionality or fixes.
* [ ] This PR contains the link to the Jira ticket it addresses.

#### Configs and Secrets

* [ ] No new CONFIG variables are required -OR- the new required ones were added to the helmchart's [`values.yaml`] file.
* [ ] No new CONFIG variables are required -OR- the new required ones were added to the deployments ([`pr-preview`], [`dev`], [`demo`], `prd`).
* [ ] No new SECRETS variables are required -OR- the new required ones were mentioned in the helmchart's [`values.yaml`] file.
* [ ] No new SECRETS variables are required -OR- the new required ones were added to the deployments ([`pr-preview secrets`], [`dev secrets`], [`demo secrets`], `prd secrets`).

#### Release

* [ ] This is not a breaking change.
* [ ] **This is ready for production.**. If your PR is not ready for production, please consider opening additional complementary PRs using this one as the base. Only merge this into `develop` or `main` after it's ready for production!

#### Deployment

* [ ] Does the deployment work after merging?

[`values.yaml`]: ../helmchart/sdp/values.yaml
[`pr-preview`]: https://github.com/stellar/kube/blob/d3e4f5dd8aa4c13b45a31a5a937f3e98841171a7/kube001-dev/namespaces/common-previews/stellar-disbursement-platform/backend-helm-values
[`dev`]: https://github.com/stellar/kube/blob/d3e4f5dd8aa4c13b45a31a5a937f3e98841171a7/kube001-dev/namespaces/stellar-disbursement-platform/backend-helm-values
[`demo`]: https://github.com/stellar/kube/blob/d3e4f5dd8aa4c13b45a31a5a937f3e98841171a7/kube001-dev/namespaces/stellar-disbursement-platform/demo/demo-backend-helm-values
[`pr-preview secrets`]: https://github.com/stellar/kube/blob/d3e4f5dd8aa4c13b45a31a5a937f3e98841171a7/kube001-dev/namespaces/common-previews/externalsecrets-common-previews.yaml#L241-L346
[`dev secrets`]: https://github.com/stellar/kube/blob/d3e4f5dd8aa4c13b45a31a5a937f3e98841171a7/kube001-dev/namespaces/stellar-disbursement-platform/stellar-disbursement-platform-externalsecrets.yaml
[`demo secrets`]: https://github.com/stellar/kube/blob/d3e4f5dd8aa4c13b45a31a5a937f3e98841171a7/kube001-dev/namespaces/stellar-disbursement-platform/demo/demo-sdp-externalsecrets.yaml
