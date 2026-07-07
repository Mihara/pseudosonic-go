#!/bin/bash

# Usage: script <input-file> <output-file>
# - MP3 with ID3v2.4 → convert to ID3v2.3
# - FLAC → copy audio stream
# - For both, remove ALL metadata keys that start with "lyrics" or "comment" (case‑insensitive)
#   because Echo likes to choke on them.
# - Other files are copied unmodified.
# Source files is renamed rather than copied whenever no changes are to occur.

set -euo pipefail

input="$1"
output="$2"

# ------------------------------------------------------------
# Helper: return a list of '-metadata key=value' arguments
# for all tags except those starting with lyrics* or comment*
# (case‑insensitive). Uses jq to parse ffprobe JSON.
# ------------------------------------------------------------
get_filtered_metadata() {
	local file="$1"

	# Use ffprobe in JSON mode to handle multi‑line values correctly.
	ffprobe -v quiet -show_entries format_tags:stream_tags -of json "$file" 2>/dev/null |
		jq -r '.format.tags // {}, .streams[]?.tags // {} | to_entries[] | [.key, .value] | @tsv' |
		while IFS=$'\t' read -r key value; do
			# Convert key to lowercase for case‑insensitive match
			key_lower=$(echo "$key" | tr '[:upper:]' '[:lower:]')
			# Skip if key starts with "lyrics" or "comment"
			if [[ "$key_lower" != lyrics* && "$key_lower" != comment* ]]; then
				# Output as two lines: "-metadata" and "key=value"
				printf '%s\n' "-metadata" "$key=$value"
			fi
		done
}

# ------------------------------------------------------------
# Build the metadata argument array only if ffprobe and jq succeed.
# If they fail, meta_args stays empty and we skip clearing metadata.
# ------------------------------------------------------------
meta_args=()
if temp=$(get_filtered_metadata "$input" 2>/dev/null); then
	# Read the output line by line into the array
	while IFS= read -r line; do
		meta_args+=("$line")
	done <<<"$temp"
fi

# ------------------------------------------------------------
# Case‑insensitive extension
# ------------------------------------------------------------
ext="${input##*.}"
ext_lower="${ext,,}"

# ------------------------------------------------------------
# FLAC handling
# ------------------------------------------------------------
if [[ "$ext_lower" == "flac" ]]; then
	temp=$(mktemp) || exit 1

	# If we have metadata to re-add, use -map_metadata -1; otherwise just copy.
	if [[ ${#meta_args[@]} -gt 0 ]]; then
		if ffmpeg -y -i "$input" -codec copy -map_metadata -1 \
			-map_metadata:s:a 0:s:a \
			"${meta_args[@]}" \
			-f flac "$temp" 2>/dev/null; then
			mv -- "$temp" "$output"
		else
			rm -f "$temp"
			mv -- "$input" "$output"
		fi
	else
		# No tags to keep (or extraction failed) – just copy to avoid data loss.
		mv -- "$input" "$output"
	fi
	exit 0
fi

# ------------------------------------------------------------
# Non‑MP3: copy as‑is
# ------------------------------------------------------------
if [[ "$ext_lower" != "mp3" ]]; then
	mv -- "$input" "$output"
	exit 0
fi

# ------------------------------------------------------------
# MP3: detect ID3v2.4
# ------------------------------------------------------------
header=$(head -c 4 "$input" 2>/dev/null | od -An -tx1 | tr -d ' \n')
if [ "$header" != "49443304" ]; then
	mv -- "$input" "$output"
	exit 0
fi

# ------------------------------------------------------------
# MP3: convert to ID3v2.3, strip all lyrics* and comment*
# ------------------------------------------------------------
temp=$(mktemp) || exit 1

if [[ ${#meta_args[@]} -gt 0 ]]; then
	if ffmpeg -y -i "$input" -codec copy -map_metadata -1 \
		"${meta_args[@]}" \
		-id3v2_version 3 -write_id3v1 0 -f mp3 "$temp" 2>/dev/null; then
		mv -- "$temp" "$output"
	else
		rm -f "$temp"
		mv -- "$input" "$output"
	fi
else
	# No tags to keep – fallback to simple copy.
	mv -- "$input" "$output"
fi

exit 0
