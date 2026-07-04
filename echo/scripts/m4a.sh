#!/bin/bash

# Transcode arbitrary input audio files (anything ffmpeg understands)
# into AAC M4A, keeping as much metadata in as possible.
#
# Notice this is still not perfect: e.g. files with more than two streams
# will be transcoded correctly and fail to play on Echo/Echo Mini.

BITRATE="${3}k"

input="$1"
output="$2"

# Ensure temporary file cleanup
cleanup() {
	[ -n "$tmp_cover" ] && [ -f "$tmp_cover" ] && rm -f "$tmp_cover"
}
trap cleanup EXIT

# Check if a video stream (cover art) exists
has_video=$(ffprobe -v quiet -select_streams v -show_entries stream=index -of csv=p=0 "$input" 2>/dev/null | wc -l)

if [ "$has_video" -gt 0 ]; then
	# Get the codec of the first video stream
	codec=$(ffprobe -v quiet -select_streams v:0 -show_entries stream=codec_name -of default=noprint_wrappers=1:nokey=1 "$input" 2>/dev/null)

	tmp_cover="$(mktemp).jpg"

	if [ "$codec" = "mjpeg" ]; then
		# Already JPEG – copy without re‑encoding
		ffmpeg -v quiet -i "$input" -an -vcodec copy "$tmp_cover" -y 2>/dev/null
	else
		# Re‑encode to JPEG with high quality
		ffmpeg -v quiet -i "$input" -an -c:v mjpeg -q:v 2 "$tmp_cover" -y 2>/dev/null
	fi

	if [ $? -eq 0 ] && [ -s "$tmp_cover" ]; then
		# Attach the cover as a video stream
		ffmpeg -v quiet -i "$input" -i "$tmp_cover" \
			-map 0:a:0 \
			-map 1 \
			-c:a aac -b:a "$BITRATE" \
			-c:v copy \
			-disposition:v:0 attached_pic \
			-metadata:s:v:0 comment="Cover (front)" \
			-map_metadata 0:g \
			-map_metadata 0:s:a \
			-movflags +faststart \
			-y "$output" 2>/dev/null
		status=$?
	else
		# Fallback: audio only
		ffmpeg -v quiet -i "$input" \
			-map 0:a:0 \
			-c:a aac -b:a "$BITRATE" \
			-map_metadata 0:g \
			-map_metadata 0:s:a \
			-movflags +faststart \
			-y "$output" 2>/dev/null
		status=$?
	fi
else
	# No cover – audio only
	ffmpeg -v quiet -i "$input" \
		-map 0:a:0 \
		-c:a aac -b:a "$BITRATE" \
		-map_metadata 0:g \
		-map_metadata 0:s:a \
		-movflags +faststart \
		-y "$output" 2>/dev/null
	status=$?
fi

exit $status
