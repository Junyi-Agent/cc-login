#!/bin/sh
# install.sh: download the latest cc-login release for this machine, check
# it against the release's checksums, and install it (ADR 0016).
#
# Usage: install.sh install binary [DIR]    DIR instead of ~/.local/bin
#
#   curl -fsSL https://raw.githubusercontent.com/Junyi-Agent/cc-login/main/install.sh | sh -s -- install binary
set -eu

repo=Junyi-Agent/cc-login

die() {
	echo "install.sh: $*" >&2
	exit 1
}

usage() {
	echo "usage: install.sh install binary [DIR]    (curl -fsSL .../install.sh | sh -s -- install binary [DIR])"
}

case "${1:-} ${2:-}" in
"install binary") shift 2 ;;
"-h "* | "--help "* | "help "*)
	usage
	exit 0
	;;
*)
	usage >&2
	exit 2
	;;
esac
[ "$#" -le 1 ] || die "too many arguments; $(usage)"
dir=${1:-$HOME/.local/bin}

# The archive for this OS and CPU, as goreleaser names it.
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in
darwin | linux) ;;
*) die "no cc-login release for $os; build it with: go install github.com/$repo/cmd/cc-login@latest" ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "no cc-login release for $(uname -m); build it with: go install github.com/$repo/cmd/cc-login@latest" ;;
esac

archive=cc-login_${os}_${arch}.tar.gz
url=https://github.com/$repo/releases/latest/download

# Download and check.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/$archive" "$url/$archive" || die "could not download $url/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$url/checksums.txt" || die "could not download $url/checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
[ -n "$want" ] && [ "$want" = "$got" ] || die "$archive does not match checksums.txt; nothing installed"

# Install.
tar -xzf "$tmp/$archive" -C "$tmp" cc-login
mkdir -p "$dir"
mv -f "$tmp/cc-login" "$dir/cc-login"
chmod 0755 "$dir/cc-login"
echo "Installed $("$dir/cc-login" --version) in $dir"

case ":$PATH:" in
*":$dir:"*) ;;
*) echo "$dir is not in PATH; add it in ~/.zshrc: export PATH=\"$dir:\$PATH\"" ;;
esac
