#!/bin/sh
# termino installer: downloads a release archive from GitHub, verifies its
# sha256 against the release checksums.txt and installs the binary.
#
#   curl -fsSL https://raw.githubusercontent.com/lesquel/termino/main/install.sh | sh
#
# Environment:
#   TERMINO_VERSION      release tag to install (default: latest), e.g. v0.1.0
#   TERMINO_INSTALL_DIR  target directory (default: $HOME/.local/bin)
set -eu

REPO="lesquel/termino"
INSTALL_DIR="${TERMINO_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${TERMINO_VERSION:-latest}"

err() {
	echo "termino-install: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"
}

fetch() {
	# fetch <url> <output-file>
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		err "curl or wget is required"
	fi
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		err "sha256sum or shasum is required to verify the download"
	fi
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	*) err "unsupported OS: $(uname -s) (Windows: download the zip from the releases page)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) err "unsupported architecture: $(uname -m)" ;;
	esac
}

resolve_version() {
	if [ "$VERSION" != "latest" ]; then
		echo "$VERSION"
		return
	fi
	tmp_json="$1/latest.json"
	fetch "https://api.github.com/repos/$REPO/releases/latest" "$tmp_json" ||
		err "could not query the latest release"
	tag=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1)
	[ -n "$tag" ] || err "could not determine the latest release tag"
	echo "$tag"
}

main() {
	need uname
	need tar
	need awk
	need sed

	os=$(detect_os)
	arch=$(detect_arch)

	workdir=$(mktemp -d)
	trap 'rm -rf "$workdir"' EXIT INT TERM

	tag=$(resolve_version "$workdir")
	version=${tag#v}
	archive="termino_${version}_${os}_${arch}.tar.gz"
	base="https://github.com/$REPO/releases/download/$tag"

	echo "Downloading termino $tag ($os/$arch)..."
	fetch "$base/$archive" "$workdir/$archive" || err "download failed: $base/$archive"
	fetch "$base/checksums.txt" "$workdir/checksums.txt" || err "download failed: $base/checksums.txt"

	expected=$(awk -v f="$archive" '$2 == f {print $1}' "$workdir/checksums.txt")
	[ -n "$expected" ] || err "$archive is not listed in checksums.txt"
	actual=$(sha256_of "$workdir/$archive")
	if [ "$expected" != "$actual" ]; then
		err "checksum mismatch for $archive (expected $expected, got $actual); refusing to install"
	fi
	echo "Checksum verified."

	tar -xzf "$workdir/$archive" -C "$workdir" termino || err "archive does not contain the termino binary"

	mkdir -p "$INSTALL_DIR"
	cp "$workdir/termino" "$INSTALL_DIR/termino.tmp"
	chmod 0755 "$INSTALL_DIR/termino.tmp"
	mv "$INSTALL_DIR/termino.tmp" "$INSTALL_DIR/termino"

	echo "Installed termino $tag to $INSTALL_DIR/termino"
	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*) echo "Note: $INSTALL_DIR is not in your PATH. Add it, then run: termino setup" ;;
	esac
}

main "$@"
