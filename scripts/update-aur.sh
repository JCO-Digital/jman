#!/bin/bash

# This script updates the PKGBUILD with the latest version and SHA256 sum.
# It is intended to be run during the GitHub Action release workflow.

set -e

VERSION=$1
if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

# Remove 'v' prefix if present
VERSION=${VERSION#v}

AUR_DIR="aur"
PKGBUILD="${AUR_DIR}/PKGBUILD"

if [ ! -f "$PKGBUILD" ]; then
    echo "Error: PKGBUILD not found at $PKGBUILD"
    exit 1
fi

# Download the source tarball to calculate the checksum
SOURCE_URL="https://github.com/JCO-Digital/jman/archive/refs/tags/v${VERSION}.tar.gz"
TEMP_FILE=$(mktemp)

echo "Downloading ${SOURCE_URL}..."
curl -sL "$SOURCE_URL" -o "$TEMP_FILE"

SHA256=$(sha256sum "$TEMP_FILE" | awk '{ print $1 }')
rm "$TEMP_FILE"

echo "Updating PKGBUILD to version ${VERSION} with SHA256 ${SHA256}"

# Update pkgver
sed -i "s/^pkgver=.*/pkgver=${VERSION}/" "$PKGBUILD"
# Reset pkgrel to 1 for new versions
sed -i "s/^pkgrel=.*/pkgrel=1/" "$PKGBUILD"
# Update sha256sums
sed -i "s/^sha256sums=('.*')/sha256sums=('${SHA256}')/" "$PKGBUILD"

echo "PKGBUILD updated successfully."
