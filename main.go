package main

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/alitto/pond/v2"
	"github.com/supersonic-app/go-subsonic/subsonic"
	"gopkg.in/ini.v1"
)

type LyricsMode string

const (
	LyricsModeNone    = "no"
	LyricsModeFlat    = "flat"
	LyricsModeOmit    = "omit"
	LyricsModeText    = "text"
	LyricsModeTextTxt = "text.txt"
)

func fetchProfile(
	profile *ini.Section,
	client *subsonic.Client,
) map[string]*subsonic.Child {

	songs := make(map[string]*subsonic.Child)

	// These are made a lot easier by closures.
	fetchSong := func(song *subsonic.Child) {
		if song.IsDir || song.IsVideo {
			return
		}
		songs[song.ID] = song
	}

	// I don't need to remember information about individual albums with
	// the Go API: song objects contain everything I need.
	fetchAlbum := func(album *subsonic.AlbumID3) {
		for _, song := range album.Song {
			fetchSong(song)
		}
	}

	playlist := profile.Key("playlist").String()
	if playlist == "" {
		log.Println("collecting favorites...")

		// The three kinds of favorites.
		favorites, err := client.GetStarred2(map[string]string{})
		if err != nil {
			log.Fatalf("failed when getting favorites: %v", err)
		}

		// Starred artists mean we download completely
		// every album they are linked to,
		// except if the album is flagged as compilation,
		// since that can and probably does contain
		// lots of other artists.
		for _, artist := range favorites.Artist {
			for _, album := range artist.Album {
				if !album.IsCompilation {
					fetchAlbum(album)
				}
			}
		}

		// Starred albums and songs, however, are unambiguous.
		for _, album := range favorites.Album {
			fetchAlbum(album)
		}
		for _, song := range favorites.Song {
			fetchSong(song)
		}

	} else {

		log.Printf("collecting playlist '%s'", playlist)

		playlists, err := client.GetPlaylists(map[string]string{})
		if err != nil {
			log.Fatalf("failed when requesting playlists: %v", err)
		}

		for _, playlistItem := range playlists {
			if playlistItem.Name == playlist {
				thatPlaylist, err := client.GetPlaylist(playlistItem.ID)
				if err != nil {
					log.Fatalf("failed when retreiving playlist %s: %v", playlist, err)
				}
				for _, song := range thatPlaylist.Entry {
					fetchSong(song)
				}
				break
			}
		}

	}

	return songs
}

func downloadSongs(
	profile *ini.Section,
	songs map[string]*subsonic.Child,
	client *subsonic.Client,
	poolSize int,
	lyricMode LyricsMode,
	forceOverwrite bool,
) {
	// So, new rules.

	// if max_bitrate is a number greater than 0,
	// then songs that do not exceed max_bitrate
	// and are in one of the supported_formats
	// do not get transcoded.

	// Otherwise, working as before, i.e. transcode
	// into the given format and bitrate.

	var err error

	overwrite := profile.Key("overwrite").MustBool(false) || forceOverwrite
	coverArt := profile.Key("coverart").MustBool(false)

	coverArtSize := profile.Key("coverart_size").MustInt(512)
	coverArtFile := profile.Key("coverart_file").MustString("cover.jpg")
	coverSquare := profile.Key("coverart_square").MustBool(false)

	targetFormat := profile.Key("format").MustString("mp3")
	targetBitrate := profile.Key("bitrate").MustInt(128)

	log.Printf(
		"target format: %s, target bitrate: %d kbps, overwrite existing: %t", targetFormat, targetBitrate, overwrite,
	)

	log.Printf("save cover art: %t", coverArt)

	if coverArt {
		log.Printf(
			"coerce cover to square: %t, cover art filename: %s, max size: %d px",
			coverSquare, coverArtFile, coverArtSize,
		)
	}

	maxBitrate := 0
	supportedFormats := []string{}

	maxBitrate = profile.Key("max_bitrate").MustInt(0)
	supportedFormats = profile.Key("supported_formats").Strings(",")

	log.Printf("maximum untouched bitrate: %d kbps", maxBitrate)

	if maxBitrate > 0 {
		log.Printf(
			"supported formats: %s.",
			strings.Join(supportedFormats, ", "),
		)
	}

	log.Printf("lyrics processing mode: %s", lyricMode)

	// Iterate in sorted order.
	// We actually only needed a map in case
	// a song makes it into the list for two separate reasons,
	// by the time we're here we use it as a slice.
	sortedSongs := slices.SortedFunc(maps.Values(songs),
		func(a, b *subsonic.Child) int {
			return cmp.Compare(a.Path, b.Path)
		})

	// Create a pool for our work.
	pool := pond.NewPool(poolSize)
	group := pool.NewGroup()

	// List of files already written,
	// specifically applies to covers, which
	// multiple songs per album may trigger the write of.
	// Should any other per-x thing emerge, like artist.jpg,
	// it will need to use the same locking.
	var writtenFiles sync.Map

	for _, song := range sortedSongs {

		group.SubmitErr(func() error {

			songPath := filepath.Join(
				profile.Key("music_dir").String(),
				legalize(song.DisplayAlbumArtist),
				legalize(song.Album),
			)

			if err = os.MkdirAll(songPath, 0775); err != nil {
				log.Fatalf("could not create directory %s: %v", songPath, err)
			}

			// The condition whether the song will be downloaded
			// as is or transcoded:
			passthrough := maxBitrate > 0 &&
				song.BitRate <= maxBitrate &&
				slices.Contains(supportedFormats, song.Suffix)

			ext := targetFormat

			if passthrough {
				ext = song.Suffix
			}

			songBaseName := legalize(fmt.Sprintf(
				"%02d-%02d %s",
				song.DiscNumber,
				song.Track,
				song.Title,
			))

			songFileName := fmt.Sprintf("%s.%s", songBaseName, ext)

			songFile := filepath.Join(songPath, songFileName)

			if overwrite || !fileExists(songFile) {

				var rc io.ReadCloser

				if passthrough {

					log.Printf("downloading %s\n", songFile)
					rc, err = client.Download(song.ID)

				} else {

					log.Printf("transcoding %s\n", songFile)
					rc, err = client.Stream(song.ID, map[string]string{
						"maxBitRate": strconv.Itoa(targetBitrate),
						"format":     targetFormat,
					})

				}

				if err = saveToFile(rc, songFile); err != nil {
					log.Printf("failed to write song to file %s: %v", songFile, err)
					return err
				}
			}

			// Write lyrics if available and configured to.
			if lyricMode != LyricsModeNone {

				lyricsFileNameLrc := filepath.Join(songPath,
					songBaseName+".lrc")
				lyricsFileNameTxt := filepath.Join(songPath,
					songBaseName+".txt")

				if overwrite ||
					(!fileExists(lyricsFileNameLrc) &&
						!fileExists(lyricsFileNameTxt)) {

					lyricslist, err := client.GetLyricsBySongId(song.ID)
					if err != nil {
						log.Printf(
							"failed when requesting song lyrics for %s: %v",
							songBaseName, err,
						)
						return err
					}

					if len(lyricslist.StructuredLyrics) > 0 {
						var lyricsBuffer bytes.Buffer

						// Now let's try to rebuild them into an lrc file.
						// For now, ignore all responses but the first,
						// as I'm not sure navidrome even can return more
						// than one.
						chunk := lyricslist.StructuredLyrics[0]

						switch {
						case chunk.Synced || lyricMode == LyricsModeFlat:

							if chunk.DisplayTitle != "" {
								fmt.Fprintf(&lyricsBuffer,
									"[ti:%s]\n",
									chunk.DisplayTitle,
								)
							}
							if chunk.DisplayArtist != "" {
								fmt.Fprintf(&lyricsBuffer,
									"[ar:%s]\n",
									chunk.DisplayArtist,
								)
							}

							for _, line := range chunk.Lines {
								lyricsBuffer.WriteString(
									LRCStamp(line.Start + chunk.Offset),
								)
								lyricsBuffer.WriteString(line.Text)
								lyricsBuffer.WriteString("\n")
							}

						case !chunk.Synced && lyricMode == LyricsModeOmit:
						// Do nothing if the mode is omit and the lyric is not timed.
						case !chunk.Synced &&
							(lyricMode == LyricsModeText ||
								lyricMode == LyricsModeTextTxt):
							for _, line := range chunk.Lines {
								lyricsBuffer.WriteString(line.Text)
								lyricsBuffer.WriteString("\n")
							}
						}

						if lyricsBuffer.Len() > 0 {

							finalFilename := lyricsFileNameLrc
							if !chunk.Synced && lyricMode == LyricsModeTextTxt {
								finalFilename = lyricsFileNameTxt
							}

							log.Printf("saving %s\n", finalFilename)
							saveToFile(
								io.NopCloser(&lyricsBuffer),
								finalFilename,
							)
						}
					}
				}
			}

			// Now handle cover art.

			if coverArt {
				coverFilename := filepath.Join(songPath, coverArtFile)

				_, alreadyWritten := writtenFiles.LoadOrStore(
					coverFilename, true,
				)

				if !alreadyWritten && (overwrite || !fileExists(coverFilename)) {
					img, err := client.GetCoverArt(song.AlbumID, map[string]string{
						"size": strconv.Itoa(coverArtSize),
					})
					if err != nil {
						log.Printf("failed to get cover art image for %s: %v", songFile, err)
						return err
					}

					log.Printf("saving %s", coverFilename)

					if err := saveToImage(img, coverFilename, coverSquare); err != nil {
						log.Printf("failed to save cover art image for %s: %v", songFile, err)
						return err
					}

				}
			}

			return nil
		})

	}

	err = group.Wait()
	if err != nil {
		log.Fatalf("aborting due to error: %v", err)
	}
}

func main() {

	configFile := flag.String("config", "config.ini", "Configuration file to use.")
	showHelp := flag.Bool("h", false, "Show this help message")

	forceOverwrite := flag.Bool("o", false, "Force overwrite on this run")

	// Customize usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [arguments]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nArguments:\n  * If arguments are present, they are interpreted as profile names to be executed.\n  * If no arguments are present, all profiles will be executed.")

	}

	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	profileNames := flag.Args()

	cfg, err := ini.Load(*configFile)

	if err != nil {
		log.Fatalf("faled to read config file: %v\n", err)
	}

	baseUrl := getRequiredKey(cfg, "SERVER", "base_url")
	username := getRequiredKey(cfg, "SERVER", "username")
	password := getRequiredKey(cfg, "SERVER", "password")

	log.Printf("connecting to %s as %s", baseUrl, username)

	client := subsonic.Client{
		Client:     &http.Client{},
		BaseUrl:    baseUrl,
		User:       username,
		ClientName: "pseudosonic-go",
	}

	if err := client.Authenticate(password); err != nil {
		log.Fatalf("auth error: %v\n", err)
	}

	ping, err := client.Ping()
	if err != nil {
		log.Fatalf("server ping failed: %v\n", err)
	} else {
		log.Printf("server version: %s, status: %s\n", ping.Version, ping.Status)
	}

	lyricsSupported := false

	// If err is not nil, extensions are not supported.
	if extensions, err := client.GetOpenSubsonicExtensions(); err == nil {
		if slices.IndexFunc(extensions,
			func(e *subsonic.OpenSubsonicExtension) bool {
				return e.Name == "songLyrics"
			}) >= 0 {
			lyricsSupported = true
		}
	}

	log.Printf("has lyrics support: %t\n", lyricsSupported)

	poolSize := cfg.Section("SERVER").Key("workers").MustInt(
		runtime.NumCPU(),
	)

	log.Printf("number of simultaneous download/transcode tasks: %d", poolSize)

	if *forceOverwrite {
		log.Printf("overwrite forced to true for all profiles")
	}

	for _, profile := range cfg.Sections() {
		if profile.Name() == "SERVER" || profile.Name() == "DEFAULT" {
			continue
		}

		if len(profileNames) > 0 &&
			slices.Index(profileNames, profile.Name()) == -1 {
			continue
		}

		// When running a default loop, skip profiles that are default no.
		if len(profileNames) == 0 && !profile.Key("default").MustBool(true) {
			continue
		}

		log.Printf("executing profile: %s\n", profile.Name())

		songs := fetchProfile(profile, &client)

		var doLyrics LyricsMode = LyricsModeNone
		if lyricsSupported {
			profileField := profile.Key("lrc_lyrics").MustString("no")
			if profileField != "no" {
				doLyrics = LyricsModeFlat
			}
			for _, ls := range []LyricsMode{
				LyricsModeNone,
				LyricsModeOmit,
				LyricsModeFlat,
				LyricsModeText,
				LyricsModeTextTxt,
			} {
				if profileField == string(ls) {
					doLyrics = ls
					break
				}
			}
		}

		downloadSongs(profile, songs, &client,
			poolSize, doLyrics, *forceOverwrite)

	}

	log.Println("all done")

}
