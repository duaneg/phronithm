#!/bin/sh
# check-pin.sh: fail unless lsf-runtime/ (but for dist/ and the launcher)
# is unchanged between the pinned release's commit and HEAD, so the
# release the launcher downloads is the tree's build.
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
commit=$(awk '$1 == "commit" { print $2 }' "$here/release")
if [ -z "$commit" ]; then
	echo "check-pin: no release pinned in $here/release" >&2
	exit 1
fi
cd "$here/../.."
if ! git cat-file -e "$commit^{commit}" 2>/dev/null; then
	echo "check-pin: the pinned commit $commit is not in this repository; fetch more history" >&2
	exit 1
fi
if ! git diff --quiet "$commit" HEAD -- lsf-runtime ':!lsf-runtime/dist' ':!lsf-runtime/lsf-runtime'; then
	echo "check-pin: lsf-runtime/ changed since the pinned release's commit $commit; tag a release and run dist/pin.sh" >&2
	git diff --stat "$commit" HEAD -- lsf-runtime ':!lsf-runtime/dist' ':!lsf-runtime/lsf-runtime' >&2
	exit 1
fi
