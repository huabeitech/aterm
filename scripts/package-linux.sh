#!/usr/bin/env bash
set -euo pipefail

arch="$(dpkg --print-architecture)"
test "$arch" = "${1:?Expected the target architecture}"
test "$(go env GOARCH)" = "$arch"

metadata() {
  python3 scripts/sync-build-metadata.py --print-field "$1"
}

app_name="$(metadata productName)"
package_name="${app_name,,}"
version="$(metadata version)"
description="$(metadata description)"
maintainer="$(metadata companyName)"

mkdir -p build/artifacts
staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT

chmod 755 "$staging"
install -d "$staging/DEBIAN" "$staging/usr/bin" "$staging/usr/share/applications" \
  "$staging/usr/share/pixmaps"
go build -tags production -trimpath -buildvcs=false -ldflags '-w -s' -o "$staging/usr/bin/$package_name" .
install -m 644 build/appicon.png "$staging/usr/share/pixmaps/$package_name.png"

cat > "$staging/DEBIAN/control" <<CONTROL
Package: $package_name
Version: $version
Architecture: $arch
Section: net
Priority: optional
Maintainer: $maintainer
Depends: libgtk-4-1, libwebkitgtk-6.0-4, libx11-6
Description: $description
CONTROL

cat > "$staging/usr/share/applications/$package_name.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=$app_name
Exec=$package_name
Icon=$package_name
Categories=Network;TerminalEmulator;
Terminal=false
DESKTOP

package="${app_name}-linux-${arch}-${GITHUB_SHA:0:12}.deb"
dpkg-deb --root-owner-group --build "$staging" "build/artifacts/$package"
(cd build/artifacts && sha256sum "$package" > "$package.sha256")
