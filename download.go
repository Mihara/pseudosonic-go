package main

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"log"
	"maps"
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

func fetchSongList(
	profile *ini.Section,
	client *subsonic.Client,
) ([]*subsonic.Child, string) {

	playlist := profile.Key("playlist").String()

	// The most used branch, favorites.
	if playlist == "" {

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

		// Return a sorted list: Favorites cannot have an order
		sortedSongs := slices.SortedFunc(maps.Values(songs),
			func(a, b *subsonic.Child) int {
				return cmp.Compare(a.Path, b.Path)
			})

		return sortedSongs, ""

	}

	// Playlists can have an order. More importantly, they
	// can legitimately have one song more than once in them.
	// So we don't involve a map.

	var songs []*subsonic.Child

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
				if !song.IsDir && !song.IsVideo {
					songs = append(songs, song)
				}
			}
			return songs, thatPlaylist.ID
		}
	}

	return nil, ""
}

func downloadProfile(
	client *subsonic.Client,
	cfg *ini.File,
	profileName string,
	lyricsSupported bool,
	forceOverwrite bool,
) {
	// So, new rules.

	// if max_bitrate is a number greater than 0,
	// then songs that do not exceed max_bitrate
	// and are in one of the supported_formats
	// do not get transcoded.

	// Otherwise, working as before, i.e. transcode
	// into the given format and bitrate.

	profile := cfg.Section(profileName)

	var err error

	flattenTree := profile.Key("flatten").MustBool(false)
	log.Printf("flat tree mode: %t", flattenTree)

	overwrite := profile.Key("overwrite").MustBool(false) || forceOverwrite
	coverArt := profile.Key("coverart").MustBool(false)

	coverArtSize := profile.Key("coverart_size").MustInt(512)
	coverArtFile := profile.Key("coverart_file").MustString("cover.jpg")
	coverSquare := profile.Key("coverart_square").MustBool(false)

	targetFormat := profile.Key("format").MustString("mp3")
	targetBitrate := profile.Key("bitrate").MustInt(128)

	// Pool size can be re-defined per profile.
	poolSize := profile.Key("workers").MustInt(
		cfg.Section("SERVER").Key("workers").MustInt(
			runtime.NumCPU(),
		),
	)

	log.Printf("number of simultaneous download/transcode tasks: %d", poolSize)

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

	var lyricMode LyricsMode = LyricsModeNone
	if lyricsSupported {
		profileField := profile.Key("lrc_lyrics").MustString("no")
		if profileField != "no" {
			lyricMode = LyricsModeFlat
		}
		for _, ls := range []LyricsMode{
			LyricsModeNone,
			LyricsModeOmit,
			LyricsModeFlat,
			LyricsModeText,
			LyricsModeTextTxt,
		} {
			if profileField == string(ls) {
				lyricMode = ls
				break
			}
		}
	}

	log.Printf("lyrics processing mode: %s", lyricMode)

	songs, rootID := fetchSongList(profile, client)

	// Create a pool for our work.
	pool := pond.NewPool(poolSize)
	group := pool.NewGroup()

	// List of files already written,
	// specifically applies to covers, which
	// multiple songs per album may trigger the write of.
	// Should any other per-x thing emerge, like artist.jpg,
	// it will need to use the same locking.
	var writtenFiles sync.Map

	for idx, song := range songs {

		group.SubmitErr(func() error {

			var songPath string

			if !flattenTree {
				songPath = filepath.Join(
					profile.Key("music_dir").String(),
					legalize(song.DisplayAlbumArtist),
					legalize(song.Album),
				)
			} else {
				songPath = profile.Key("music_dir").String()
			}

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

			var songBaseName string
			if !flattenTree {
				songBaseName = legalize(fmt.Sprintf(
					"%02d-%02d %s",
					song.DiscNumber,
					song.Track,
					song.Title,
				))
			} else {
				songBaseName = legalize(fmt.Sprintf(
					"%04d %s", idx+1,
					song.Title,
				))
			}

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

			if coverArt && !flattenTree {
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

					if err := saveToImage(img, coverFilename, coverSquare, coverArtSize); err != nil {
						log.Printf("failed to save cover art image for %s: %v", songFile, err)
						return err
					}

				}
			}

			return nil
		})

	}

	// If we have a rootID, i.e. were processing a playlist,
	// and are in flat mode, save the cover for the batch.
	if coverArt && flattenTree && rootID != "" {
		coverFilename := filepath.Join(profile.Key("music_dir").String(), coverArtFile)
		img, err := client.GetCoverArt(rootID, map[string]string{
			"size": strconv.Itoa(coverArtSize),
		})
		if err != nil {
			log.Printf("failed to get cover art image for profile: %v", err)
		}

		if img != nil {
			log.Printf("saving %s", coverFilename)

			if err := saveToImage(img, coverFilename, coverSquare, coverArtSize); err != nil {
				log.Fatalf("failed to save cover art image for profile: %v", err)
			}
		}
	}

	err = group.Wait()
	if err != nil {
		log.Fatalf("aborting due to error: %v", err)
	}
}
