#!/bin/bash

# Check if the script is running in GitHub Actions
if [ -n "$GITHUB_ACTIONS" ]; then
  ERROR_COLOR="::error::"
  SUCCESS_COLOR="::debug::"
else
  ERROR_COLOR='\033[0;31m'
  SUCCESS_COLOR='\033[0;32m'
fi

# Find all .go files excluding paths containing 'mock' and run goimports
non_compliant_files=$(find . -type f -name "*.go" ! -path "*mock*" | xargs goimports -local "github.com/stellar/stellar-disbursement-platform-backend" -l)

if [ -n "$non_compliant_files" ]; then
  echo -e "${ERROR_COLOR}The following files are not compliant with goimports:\n"
  echo -e "${ERROR_COLOR}${non_compliant_files}\n"
  exit 1
else
  echo -e "${SUCCESS_COLOR}All files are compliant with goimports.\n"
  exit 0
fi
