#!/bin/sh
# sync_daemonpb.sh — вендорит сгенерированные protobuf/gRPC-стабы протокола
# daemon.StartedService из форка sing-box-lx в internal/daemonpb.
#
# Зачем вендорить, а не импортировать модуль форка: go.mod лаунчера не тянет
# весь дедовский граф зависимостей sing-box (важно для win7-сборки и размера);
# стабам нужны только google.golang.org/{grpc,protobuf}.
#
# Каждому файлу добавляется `//go:build darwin`: протокол используется только
# daemon-режимом (macOS), остальные платформы его не компилируют.
#
# Usage: scripts/sync_daemonpb.sh [path-to-sing-box-lx]
set -eu

FORK="${1:-../sing-box-lx}"
DEST="internal/daemonpb"
FILES="started_service.pb.go started_service_grpc.pb.go managed_service.pb.go managed_service_grpc.pb.go"

if [ ! -d "$FORK/daemon" ]; then
    echo "sync_daemonpb: fork not found at $FORK (pass the path as \$1)" >&2
    exit 1
fi

mkdir -p "$DEST"
for f in $FILES; do
    src="$FORK/daemon/$f"
    if [ ! -f "$src" ]; then
        echo "sync_daemonpb: missing $src" >&2
        exit 1
    fi
    {
        echo "//go:build darwin"
        echo ""
        cat "$src"
    } >"$DEST/$f"
    echo "synced $f"
done

# Пришиваем ревизию форка для трассируемости.
if rev="$(git -C "$FORK" rev-parse HEAD 2>/dev/null)"; then
    echo "$rev" >"$DEST/SYNC_REV"
    echo "fork revision: $rev"
fi
