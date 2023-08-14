# How to contribute

👍🎉 First off, thanks for taking the time to contribute! 🎉👍

Check out the [Stellar Contribution Guide](https://github.com/stellar/.github/blob/master/CONTRIBUTING.md) that applies to all Stellar projects.

## Style guides

### Issues

* Ensure the issue was not already reported by searching on GitHub under Issues.
* Issues start with:
    * The functional area most affected, ex. `disbursements: fix... `.
    * Or, `ci:` when changes or an issue are isolated to CI.
    * Or, `doc:` when changes or an issue are isolated to non-code documentation not limited to a single package.
* Label issues with `bug` if they're clearly a bug.
* Label issues with `feature request` if they're a feature request.

### Pull Requests

* **Title:** PR titles start with feat, fix, refactor, ci, or doc, followed by a short description of the change.
* **Branching:** PRs must be opened against the `develop` branch.
* **Scope:** PRs must be focused and not contain unrelated commits.
* **Refactoring:** Explicitly differentiate refactoring PRs and feature PRs. Refactoring PRs don’t change functionality. They usually touch a lot more code, and are reviewed in less detail. Avoid refactoring in feature PRs.
* **Go Formatting:** Ensure your code is formatted with `gofmt`.
* **Tests:** Ensure your change is covered by tests. If you're adding a new feature or fixing a bug, you must add tests. If you're refactoring, you should add tests if possible.
* **Documentation:** Update README.md or other relevant documentation pages if necessary. For exported functions, types, and constants, make sure to add a doc comment conforming to [Effective Go](https://golang.org/doc/effective_go.html#commentary).
* **Best Practices:** * Follow [Effective Go](https://golang.org/doc/effective_go.html) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).


### Git Commit Messages

* Use the present tense ("Add feature" not "Added feature").
* Use the imperative mood ("Move cursor to..." not "Moves cursor to...").
* Start commit message with the relevant issue number, e.g., #123 Fixed bug in XYZ module.

## Development Environment

All SDP services can be started using the `docker-compose.yml` file in the `dev` directory. Please refer to the [README](dev/README.md) for more information.

## Code of Conduct

Help us keep Stellar open and inclusive. Please read and follow our [Code of Conduct](https://github.com/stellar/.github/blob/master/CODE_OF_CONDUCT.md).
