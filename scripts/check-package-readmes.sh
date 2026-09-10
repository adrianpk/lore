#!/usr/bin/env bash
set -euo pipefail

missing=0

while IFS=$'\t' read -r dir import_path; do
	if [[ ! -f "$dir/README.md" ]]; then
		printf 'missing package README: %s (%s)\n' "$dir" "$import_path" >&2
		missing=1
	fi
done < <(go list -f '{{.Dir}}{{"\t"}}{{.ImportPath}}' ./...)

exit "$missing"
