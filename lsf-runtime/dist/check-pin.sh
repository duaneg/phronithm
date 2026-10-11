#!/bin/sh
# check-pin.sh: fail unless lsf-runtime/ (but for dist/, the launcher,
# tests and testdata, none of which is in the binary) is unchanged between
# the pinned release's commit and HEAD, so the release the launcher
# downloads is the tree's build.
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
commit=$(awk '$1 == "commit" { print $2; exit }' "$here/release")
if [ -z "$commit" ]; then
	echo "check-pin: no release pinned in $here/release" >&2
	exit 1
fi
case $commit in
*[!0-9a-f]*)
	echo "check-pin: bad commit $commit in $here/release" >&2
	exit 1
	;;
esac
# glob magic: "**/" matches zero or more directories.
set -- lsf-runtime ':!lsf-runtime/dist' ':!lsf-runtime/lsf-runtime' ':(exclude,glob)lsf-runtime/**/*_test.go' ':(exclude,glob)lsf-runtime/**/testdata/**'
cd "$here/../.."
if ! git cat-file -e "$commit^{commit}" 2>/dev/null; then
	echo "check-pin: the pinned commit $commit is not in this repository; fetch more history" >&2
	exit 1
fi
if ! git diff --quiet "$commit" HEAD -- "$@"; then
	echo "check-pin: lsf-runtime/ changed since the pinned release's commit $commit; tag a release and run dist/pin.sh" >&2
	git diff --stat "$commit" HEAD -- "$@" >&2
	exit 1
fi
