#!/bin/sh
while IFS= read -r p || [ -n "$p" ]; do
  exp=".*${p}.*"
  sed -i "/${exp}/d" ./c.out
done << EOF # list of terms and files we want to exclude
mocks
EOF
