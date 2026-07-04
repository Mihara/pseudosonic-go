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

1. This has only been tested with Navidrome, and may unknowingly depend on one or another of its idiosyncrasies.
2. It's on you to configure the server to transcode into the format(s) you wish to transcode into. If you did not have a transcoding command configured for a format you planned to use, the results can be unexpected and hard to diagnose.
3. If you're using a server managed by someone else, be sure to ask them for the appropriate value for the `workers` configuration parameter, which sets how many simultaneous transcode/download jobs the program will create.
4. The program creates temporary files in the destination directories while it's working, to prevent incomplete transcodes and the like. Upon completion of the job, the files are renamed to their correct names. If you manually interrupt the program before it can finish, or if it somehow dies before it is done due to a transient network error, it's on you to delete the `*.tmp` files.

## Usage

1. Copy `config.example.ini` to `config.ini` and edit that to fit your situation. Most of the documentation is in fact in this file, since almost all of the program's functions are controlled through configuration.
2. Run and wait. The command is

```shell
pseudosonic-go [-config <config filename>] [-o] [<profile name, or several>]
```

* `-config` config option allows you to supply a specific configuration file name. By default, the program looks for `config.ini` in current directory.
* `-o` allows you to temporarily force overwrite mode for this specific run. This applies to all profiles, so beware.

The program is capable of downloading your favorited songs, or a specific named playlist, whether a smart playlist or otherwise. It is possible to have multiple "profiles" specifying how to transcode/download songs and where to put them, doing the job for multiple kinds of target player simultaneously, or selecting from one configuration file as needed. The idea is that, at least on Linux, you could configure this program to run automatically when your player is mounted as a writable device, so that it would simply add any songs you have recently favorited.

## Advanced usage

On a Linux system with systemd, you can invoke sync automatically when you plug your player in. See [autostart](autostart/) for the example systemd files and script.

Pseudosonic-go compiles and runs on Android under [Termux](https://termux.dev/), which is how I run it now for syncing Poweramp library. You can simply run the release the executable for linux-arm64 directly if you can't be bothered to compile it.

See also the [echo](echo/) directory for an example profile (and associated scripts) for a hands-off transcoding setup for FIIO Snowsky Echo. *(The requirements for Echo Mini are different.)*

## License

This little ditty is released under the terms of WTFPL 4.0
