package main

import (
	"fmt"
	"net/url"

	"github.com/supersonic-app/go-subsonic/subsonic"
)

// To avoid building a tree of json-parsing structures,
// we'll just template our request for the moment,
// it's simple enough: encode as this or else.
const transcodeRequest = `
{
  "name": "pseudosonic-go",
  "platform": "multiplatform",
  "transcodingProfiles": [
    {
      "container": "%s",
      "audioCodec": "%s",
      "protocol": "http",
      "maxAudioChannels": 2
    },
  ],
  "codecProfiles": [
    {
      "type": "AudioCodec",
      "name": "%s",
      "limitations": [
        { 
	      "name": "audioBitrate",
		  "comparison": "Equals",
		  "values": [ "%d" ],
		  "required": true
		}
      ]
    }
  ]
}
`

func forceTranscodeDecision(
	client *subsonic.Client,
	song *subsonic.Child,
	targetFormat string,
	targetBitrate int,
) {

	params := url.Values{}
	params.Add("mediaId", song.ID)
	params.Add("mediaType", song.Type)

	body := fmt.Printf(transcodeRequest,
		targetFormat, targetFormat, targetFormat,
		targetBitrate)

	resp, err := client.Request("POST", "/rest/getTranscodeDecision", params)

}
