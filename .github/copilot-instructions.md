# Copilot Code Review Instructions

## General Guidelines
- Title follows `SDP-1234: Add new feature` or `Chore: Refactor package xyz` format. The Jira ticket code was included if available.
- PR has a focused scope and doesn't mix features with refactoring
- Tests are included (if applicable. Certain changes like documentation or config updates may not need tests)
- `CHANGELOG.md` is updated 
- CONFIG/SECRETS changes are updated in helmcharts and deployments when we introduce a new configuration to the application. 

## API Changes
**If the change affects API signature (endpoints, request/response schemas, parameters), notify the developer to update the API specs at:**
https://github.com/stellar/stellar-docs/blob/main/openapi/stellar-disbursement-platform/main.yaml

## Error Handling
- Always use `errors.Is()` for error comparison, never `==` (fails with wrapped errors)
- Return errors with context: `fmt.Errorf("description: %w", err)`

## Testing
- Prefer table-driven tests pattern when possible
- Use `assert` for assertions, `require` only for critical failures that should stop the test
- Leverage test utils from `internal/data/test_utils.go`

## Security
- Validate all inputs to prevent injection
- No hardcoded secrets/credentials
- Environment variables for sensitive config