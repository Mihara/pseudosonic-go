
# Using pseudosonic-go with FIIO Snowsky Echo

Snowsky Echo has some really finicky requirements for its files, and this involved introducing new features specifically to support it -- local transcoding (i.e. transcoding files on the client to work around the streaming limitations of Navidrome) and postprocessing (massaging the downloaded files to make the tags fit.) which are described in more detail in [config.echo.ini](config.echo.ini).

While the `local_transcoder` and `postprocessor` fields should work on any platform pseudosonic-go builds for, the scripts included here are written with a unix-like system in mind, and expect `ffmpeg` to be available in path.
