#!/bin/bash
# Cross-build Remagic Home against Quill and the reMarkable ferrari SDK.
set -euo pipefail
cd "$(dirname "$0")"
QUILL=${QUILL_DIR:-../../quill}
[ -f "$QUILL/build/libquill.so" ] || (cd "$QUILL" && ./build.sh)
SDK=${RM_SDK:-$HOME/rm-sdk-3.26}
ENV=$(find "$SDK" -maxdepth 1 -name 'environment-setup-*' | head -n1)
unset LD_LIBRARY_PATH
source "$ENV"
QTINC="$SDKTARGETSYSROOT/usr/include"
mkdir -p build
$CXX -O2 \
    -I "$QTINC" -I "$QTINC/QtCore" -I "$QTINC/QtGui" \
    src/home.cpp \
    -L "$QUILL/build" -lquill \
    -L "$QUILL/vendor" -lqsgepaper \
    -lQt6Gui -lQt6Core -lstdc++ \
    -Wl,-rpath,/home/root/remagic-home \
    -o build/home
echo "built: build/home"
