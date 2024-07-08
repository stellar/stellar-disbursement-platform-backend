# Declare colrs for printing
RED='\033[0;31m'
GREEN='\033[0;32m'

# Find all .go files excluding paths containing 'mock' and run goimports
non_compliant_files=$(find . -type f -name "*.go" ! -path "*mock*" | xargs goimports -local "github.com/stellar/stellar-disbursement-platform-backend" -l)

if [ -n "$non_compliant_files" ]; then
  echo "${RED}The following files are not compliant with goimports:"
  echo "${RED}$non_compliant_files"
  exit 1
else
  echo "${GREEN}All files are compliant with goimports"
  exit 0
fi
