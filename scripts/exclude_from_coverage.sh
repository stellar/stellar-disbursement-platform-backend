#!/bin/sh

exclude_terms() {
    local terms="$1"
    local infile="$2"
    local tmpfile="${infile}.tmp"

    while IFS= read -r term || [ -n "$term" ]; do
        local exp=".*${term}.*"
        grep -v "$exp" "$infile" > "$tmpfile"
        mv "$tmpfile" "$infile"
    done << EOF
$terms
EOF
}

# Usage
exclude_terms "mock" "c.out"
exclude_terms "tss_payments_loadtest.go" "c.out"
