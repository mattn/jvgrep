#!/bin/bash

make
VERSION=`./jvgrep -V`
XC_ARCH=${XC_ARCH:-386 amd64}
XC_OS=${XC_OS:-linux darwin windows}

rm -rf pkg/
gox \
    -os="${XC_OS}" \
    -arch="${XC_ARCH}" \
    -output "pkg/{{.OS}}_{{.Arch}}/{{.Dir}}"

for ARCHIVE in ./pkg/*; do
    ARCHIVE_NAME=$(basename ${ARCHIVE})
    echo Uploading: ${ARCHIVE_NAME}
    curl \
        -T ${ARCHIVE} \
        -u mattn:${BINTRAY_API_KEY} \
        "https://api.bintray.com/content/mattn/jvgrep/jvgrep/${VERSION}/${ARCHIVE_NAME}"
    echo
done
