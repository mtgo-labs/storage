#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

sqlc generate

for dir in ../sqlite/sqlc_db.go ../postgres/sqlc_db.go; do
  sed -i 's/func New()/func NewSqlcQueries()/' "$dir"
done

echo "sqlc generate + post-process done"
