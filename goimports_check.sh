# Find all .go files excluding paths containing 'mock' and run goimports
non_compliant_files=$(find . -type f -name "*.go" ! -path "*mock*" | xargs goimports -local "github.com/stellar/stellar-disbursement-platform-backend" -l)

if [ -n "$non_compliant_files" ]; then
  printf "::error::The following files are not compliant with goimports:\n"
  printf "::error::%s\n" "$non_compliant_files"
  exit 1
else
  printf "::notice::All files are compliant with goimports.\n"
  exit 0
fi
