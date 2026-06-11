# Pseudosonic-go

Imagine that you have a [Navidrome](https://www.navidrome.org/) server, or another Subsonic API server capable of transcoding,

* And an MP3 player, not capable of running a Subsonic client or playing half the formats your music collection is stored in, which you need to synchronize a substantial subset of your music library onto.
* Or, you don't like the available Subsonic clients for your phone and wish to just make an offline copy of your library, compressed down for your phone.
* Or they do internet shutdowns regularly where you are, so you can't depend on the network to always have your music with you, and that offline copy of your library is the only way to go.

While you can, of course, go for the original files themselves, you then have the hassle of transcoding them, but only *some* of them, because there will always be some special requirement or other.

You can press your server into doing the transcoding as well as selecting files to be synchronized. But you need a Subsonic API client to do that.

This is the second iteration of my solution to this problem, a Subsonic API client meant to produce a downloaded, partially or fully transcoded copy of a selection of your music library. The original was a [quick and dirty script in Python](https://github.com/mihara/pseudosonic).

This version, rewritten from the ground up in Go, has the benefit of being much faster, easier to deploy, and adds features that never made it into the Python version.

## Caveats

1. Downloaded files are saved as an `<artist>/<album>/<disc>-<track> <song name>.<format extension>` directory tree, according to their tags, rather than directory locations on the server, which may or may not be what you wanted.
2. It's on you to configure the server to transcode into the format(s) you wish to transcode into.
3. If you're using a server ran by someone else, be sure to ask them for the appropriate value for the `workers` configuration parameter, which sets how many simultaneous transcodes the program will request.
4. The program creates temporary files in the destination directories while it's working, to prevent incomplete transcodes and the like. Upon completion of the job, the files are renamed to their correct names. If you manually interrupt the program before it can finish, it's on you to delete the `*.tmp` files.

## Usage

1. Copy `config.ini.example` to `config.ini` and edit that. Most of the documentation is in fact in this file, since almost all of the program's functions are controlled through configuration. By default, the program looks for `config.ini` in current directory, but you can supply a different filename on command line.
2. Run and wait. The command is

```shell
pseudosonic-go [-c <config filename>] [<profile name, or several>]
```

The program is capable of downloading your favorited songs, or a specific named playlist, whether a smart playlist or otherwise.

## License

This little ditty is released under the terms of WTFPL 4.0
