#!/bin/sh
while IFS= read -r p || [ -n "$p" ]; do
  exp=".*${p}.*"
  grep -v "$exp" c.out > c.out.tmp && mv c.out.tmp c.out
done << EOF # list of terms we want to exclude
mock
EOF
