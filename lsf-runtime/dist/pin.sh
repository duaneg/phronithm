#!/bin/sh
# pin.sh TAG: pin the launcher to the published release TAG, rewriting
# dist/release from the release's SHA256SUMS (gh release download).
set -eu
[ $# -eq 1 ] || { echo "usage: $0 TAG" >&2; exit 2; }
tag=$1
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
d=$(mktemp -d)
trap 'rm -rf -- "$d"' EXIT
gh release download "$tag" -R duaneg/phronithm -p SHA256SUMS -D "$d"
commit=$(git -C "$here" rev-list -n1 "$tag")
sum() { awk -v f="lsf-runtime-linux-$1" '$2 == f || $2 == "*" f { print $1 }' "$d/SHA256SUMS"; }
amd64=$(sum amd64)
arm64=$(sum arm64)
for h in "$amd64" "$arm64"; do
	case $h in
	????????????????????????????????????????????????????????????????) ;;
	*) echo "$0: SHA256SUMS of $tag lacks a linux amd64 or arm64 sum" >&2; exit 1 ;;
	esac
done
{
	sed -n '/^#/p' "$here/release"
	printf 'tag %s\ncommit %s\nsha256 amd64 %s\nsha256 arm64 %s\n' "$tag" "$commit" "$amd64" "$arm64"
} >"$d/release"
mv -f -- "$d/release" "$here/release"
cat "$here/release"
