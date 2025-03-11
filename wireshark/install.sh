#!/bin/sh

PLUGINS_DIR=$HOME/.config/wireshark/plugins
mkdir -p "$PLUGINS_DIR"

for d in *.lua; do
	ln -s "$PWD/$d" "$PLUGINS_DIR/$d"
done
