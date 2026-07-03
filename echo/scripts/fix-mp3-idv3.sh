#!/bin/bash

# If the input is an MP3 with an ID3v2.4 tag, convert it to ID3v2.3
# using ffmpeg and save to output.
# Otherwise, copy the file unmodified.

set -euo pipefail

input="$1"
output="$2"

# Case‑insensitive extension check
ext="${input##*.}"
if [[ "${ext,,}" != "mp3" ]]; then
	cp -- "$input" "$output"
	exit 0
fi

# Detect ID3v2.4: first 4 bytes must be "49 44 33 04"
header=$(head -c 4 "$input" 2>/dev/null | od -An -tx1 | tr -d ' \n')
if [ "$header" != "49443304" ]; then
	cp -- "$input" "$output"
	exit 0
fi

temp=$(mktemp) || exit 1

if ffmpeg -y -i "$input" -codec copy -map_metadata 0 \
	-id3v2_version 3 -write_id3v1 0 -f mp3 "$temp" 2>/dev/null; then
	mv -- "$temp" "$output"
else
	# Clean up, copy and cross fingers.
	rm -f "$temp"
	cp -- "$input" "$output"
	exit 0
fi
