
# Using pseudosonic-go with FIIO Snowsky Echo

Snowsky Echo has some really finicky requirements for its files, and this involved introducing new features specifically to support it -- local transcoding (i.e. transcoding files on the client to work around the streaming limitations of Navidrome) and postprocessing (massaging the downloaded files to make the tags fit.) which are described in more detail in [config.echo.ini](config.echo.ini).

While the `local_transcoder` and `postprocessor` fields should work on any platform pseudosonic-go builds for, the scripts included here are written with a unix-like system in mind, and expect `ffmpeg` to be available in path.

One annoying quirk of Echo in current (1.6.0) firmware, which it probably doesn't share with Echo Mini, and which you should know about, is that when you *alter* metadata on a file (or otherwise replace the file with a different file with the same name) the next media metadata refresh will crash thhe player. The workaround is to always delete the files you need to alter metadta on first, refresh the metadata, and then replace the file and refresh again.
