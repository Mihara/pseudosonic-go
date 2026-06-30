package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"slices"

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

		downloadSongs(profile, songs, &client,
			poolSize, lyricsSupported, *forceOverwrite)

	}

	log.Println("all done")

}
