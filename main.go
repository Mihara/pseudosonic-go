package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"

	"github.com/supersonic-app/go-subsonic/subsonic"
	"gopkg.in/ini.v1"
)

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
	transcodingControlSupported := false

	// If err is not nil, extensions are not supported.
	if extensions, err := client.GetOpenSubsonicExtensions(); err == nil {
		for _, e := range extensions {
			switch e.Name {
			case "songLyrics":
				lyricsSupported = true
			case "transcoding":
				transcodingControlSupported = true
			}
		}
	}

	log.Printf("has lyrics support: %t\n", lyricsSupported)

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

		downloadProfile(&client,
			cfg, profile.Name(),
			lyricsSupported, transcodingControlSupported,
			*forceOverwrite)

	}

	log.Println("all done")

}
