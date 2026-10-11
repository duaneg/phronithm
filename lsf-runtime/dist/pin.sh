#!/bin/sh
# pin.sh TAG: pin the launcher to the published release TAG, rewriting
# dist/release from the release's SHA256SUMS (gh release download), after
# checking the release's binaries against it. TAG must be fetched locally.
set -eu
[ $# -eq 1 ] || { echo "usage: $0 TAG" >&2; exit 2; }
tag=$1
case $tag in
*[!A-Za-z0-9._-]* | .*) echo "$0: bad tag $tag" >&2; exit 2 ;;
esac
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
d=$(mktemp -d)
new=
trap 'rm -rf -- "$d"; rm -f -- "$new"' EXIT
commit=$(git -C "$here" rev-list -n1 "refs/tags/$tag")
gh release download "$tag" -R duaneg/phronithm -p SHA256SUMS -p 'lsf-runtime-linux-*' -D "$d"
(cd "$d" && sha256sum -c --strict SHA256SUMS) >&2
sum() { awk -v f="lsf-runtime-linux-$1" '$2 == f || $2 == "*" f { print $1 }' "$d/SHA256SUMS"; }
# The binaries embed their commit (-X main.commit): it must be the tag's.
case $(uname -m) in
x86_64 | amd64) host=amd64 ;;
aarch64 | arm64) host=arm64 ;;
*) host= ;;
esac
if [ -n "$host" ]; then
	chmod 755 "$d/lsf-runtime-linux-$host"
	built=$("$d/lsf-runtime-linux-$host" version)
	if [ "$built" != "$tag $commit" ]; then
		echo "$0: the release's binary reports \"$built\"; the local tag $tag is at $commit" >&2
		exit 1
	fi
fi
amd64=$(sum amd64)
arm64=$(sum arm64)
for h in "$amd64" "$arm64"; do
	case $h in
	*[!0-9a-f]*) echo "$0: SHA256SUMS of $tag has a bad sum $h" >&2; exit 1 ;;
	????????????????????????????????????????????????????????????????) ;;
	*) echo "$0: SHA256SUMS of $tag lacks a linux amd64 or arm64 sum" >&2; exit 1 ;;
	esac
done
new=$(mktemp "$here/.release.XXXXXX")
{
	sed -n '/^#/p' "$here/release"
	printf 'tag %s\ncommit %s\nsha256 amd64 %s\nsha256 arm64 %s\n' "$tag" "$commit" "$amd64" "$arm64"
} >"$new"
chmod 644 "$new"
mv -f -- "$new" "$here/release"
new=
cat "$here/release"
