#!/bin/sh
while IFS= read -r p || [ -n "$p" ]; do
  exp=".*${p}.*"
  grep -v "$exp" c.out > c.out.tmp
done << EOF # list of terms we want to exclude
mock
tss_payments_loadtest.go
EOF

mv c.out.tmp c.out
