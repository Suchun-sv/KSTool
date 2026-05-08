#!/bin/bash
set -e

# Static build (no glibc dependency) so the binary runs on any EIDF host.
CGO_ENABLED=0 "$GOROOT/bin/go" build -ldflags="-s -w" -o kstool ./cmd/kstool

# Atomic-replace on the remote: scp to a temp name, then mv. mv just swaps
# the directory entry, so a kstool that's currently running keeps executing
# on its old inode (no ETXTBSY).
scp kstool eidf:kstool.new
ssh eidf 'mv -f kstool.new kstool && chmod +x kstool'
echo "kstool deployed to eidf:~/kstool"
