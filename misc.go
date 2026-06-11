package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	webp "github.com/HugoSmits86/nativewebp"
	"github.com/disintegration/imaging"
	"github.com/flytam/filenamify"
	"golang.org/x/image/bmp"
	"gopkg.in/ini.v1"
)

func getRequiredKey(cfg *ini.File, section, key string) string {
	value := cfg.Section(section).Key(key).String()
	if value == "" {
		log.Fatalf("missing required key in [%s]: %s\n", section, key)
	}
	return value
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err)
}

func saveToFile(rc io.ReadCloser, path string) error {
	defer rc.Close()

	if path == "" {
		return os.ErrInvalid
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmpf, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return err
	}

	tmpName := tmpf.Name()
	success := false

	defer func() {
		tmpf.Close()

		// Clean up on failure, that's what the flag is for.
		if !success {
			os.Remove(tmpName)
		}
	}()

	_, err = io.Copy(tmpf, rc)
	if err != nil {
		return err
	}

	if err := tmpf.Sync(); err != nil {
		return err
	}
	if err := tmpf.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	success = true
	return nil
}

func MakeSquare(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width == height {
		return img
	}

	side := width
	if height > width {
		side = height
	}

	log.Printf("image type: %T\n", img)

	background := imaging.New(side, side, color.Black)
	return imaging.OverlayCenter(background, img, 1.0)
}

func saveToImage(img image.Image, path string, square bool) error {
	if img == nil {
		return fmt.Errorf("image is nil")
	}
	if path == "" {
		return os.ErrInvalid
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Create temporary file in the same directory
	tmpf, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return err
	}

	tmpName := tmpf.Name()
	success := false

	defer func() {
		tmpf.Close()
		if !success {
			os.Remove(tmpName)
		}
	}()

	ext := strings.ToLower(filepath.Ext(path))
	imgToSave := img

	if square {
		imgToSave = MakeSquare(img)
	}

	var encodeErr error
	switch ext {
	case ".png":
		encodeErr = png.Encode(tmpf, imgToSave)
	case ".jpg", ".jpeg":
		encodeErr = jpeg.Encode(tmpf, imgToSave, &jpeg.Options{Quality: 95})
	case ".webp":
		encodeErr = webp.Encode(tmpf, imgToSave, &webp.Options{CompressionLevel: 6})
	case ".bmp":
		encodeErr = bmp.Encode(tmpf, imgToSave)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}

	if encodeErr != nil {
		return encodeErr
	}
	if err := tmpf.Sync(); err != nil {
		return err
	}
	if err := tmpf.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	success = true
	return nil
}

// Legalize a filename by replacing characters and sequences
// illegal in filenames by underscores and trimming the length
// down to reasonable.
func legalize(s string) string {
	// Ironically, err is always nil if the options are correct.
	ns, _ := filenamify.Filenamify(s, filenamify.Options{
		Replacement: "_",
		MaxLength:   100,
	})
	return ns
}
