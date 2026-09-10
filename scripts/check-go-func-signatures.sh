#!/usr/bin/env sh
set -eu

base_ref="${GO_FUNC_SIGNATURE_BASE_REF:-origin/dev}"

failed=0

check_diff() {
	diff_args="$1"
	current_file=""

	# shellcheck disable=SC2086
	git diff --unified=0 $diff_args -- '*.go' | while IFS= read -r line; do
		case "$line" in
			"+++ b/"*)
				current_file=${line#"+++ b/"}
				;;
			+* )
				added=${line#+}
				if printf '%s\n' "$added" | grep -Eq '^[[:space:]]*func[[:space:]].*\($'; then
					printf '%s:%s\n' "$current_file" "$added"
					exit 2
				fi
				;;
		esac
	done
}

if git rev-parse --verify "$base_ref" >/dev/null 2>&1; then
	if ! check_diff "$base_ref...HEAD"; then
		failed=1
	fi
fi

if ! check_diff ""; then
	failed=1
fi

new_files=$(git ls-files --others --exclude-standard -- '*.go')
for file in $new_files; do
	matches=$(grep -nE '^[[:space:]]*func[[:space:]].*\($' "$file" || true)
	if [ -n "$matches" ]; then
		printf '%s\n' "$matches" | while IFS= read -r match; do
			printf '%s:%s\n' "$file" "$match"
		done
		failed=1
	fi
done

if [ "$failed" -ne 0 ]; then
	printf '%s\n' "Go function signatures must keep parameters on the signature line when possible."
	exit 1
fi
