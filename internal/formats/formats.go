// Package formats reads book files: PDF (pdfium in WebAssembly), CBZ and CBR.
package formats

import (
	"bytes"
	"errors"
	"io"
	"os"
	"image"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

// Metadata is what a book file can tell about itself.
type Metadata struct {
	Title  string
	Author string
}

// BookSource is one opened book file. Close it when done.
type BookSource interface {
	Format() string // "pdf" | "cbz" | "cbr"
	PageCount() (int, error)
	Metadata() (Metadata, error)
	Cover(maxW, maxH int) (image.Image, error)
	// Page returns a comic page (0-based) as image bytes and a content type.
	Page(n int) ([]byte, string, error)
	Close() error
}

// ErrUnsupported is returned for unsupported formats or operations.
var ErrUnsupported = errors.New("unsupported")

// FormatOf returns the format for a file name, or "" if it is not a book.
func FormatOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return "pdf"
	case ".cbz":
		return "cbz"
	case ".cbr":
		return "cbr"
	}
	return ""
}

// Open opens a book file by its extension.
func Open(path string) (BookSource, error) {
	switch FormatOf(path) {
	case "pdf":
		return openPDF(path)
	case "cbz", "cbr":
		// Comic files are often misnamed (a .cbr that is really a zip), so the
		// container is detected from its magic bytes, not the extension.
		var src BookSource
		var err error
		if sniffArchive(path) == "rar" {
			src, err = openCBR(path)
		} else {
			src, err = openCBZ(path)
		}
		if err != nil {
			return nil, err
		}
		return labeled{src, FormatOf(path)}, nil
	}
	return nil, ErrUnsupported
}

// labeled reports the format by extension while reading the real container.
type labeled struct {
	BookSource
	format string
}

func (l labeled) Format() string { return l.format }
func (l labeled) Pages() []string { return l.BookSource.(Comic).Pages() }

// sniffArchive returns "rar", "zip" or "" from a file's first bytes.
func sniffArchive(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var b [8]byte
	n, _ := io.ReadFull(f, b[:])
	switch {
	case n >= 6 && bytes.Equal(b[:6], []byte("Rar!\x1a\x07")):
		return "rar"
	case n >= 4 && bytes.Equal(b[:4], []byte("PK\x03\x04")):
		return "zip"
	}
	return ""
}

// Comic is implemented by the archive formats.
type Comic interface {
	BookSource
	// Pages lists the page entry names in reading order.
	Pages() []string
}

// ReadEntry reads one named entry of a comic archive (used to serve a page
// from the entry name recorded at import).
func ReadEntry(path, entry string) ([]byte, string, error) {
	if FormatOf(path) != "cbz" && FormatOf(path) != "cbr" {
		return nil, "", ErrUnsupported
	}
	return ReadRarEntryCaching(path, entry, nil)
}

// isPageName reports whether an archive entry is a comic page image.
func isPageName(name string) bool {
	name = strings.ReplaceAll(name, "\\", "/")
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || part == "__MACOSX" {
			return false
		}
	}
	return ContentType(name) != ""
}

// ContentType returns the image content type for a page name, or "".
func ContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	return ""
}

// Fit scales img down to fit within maxW x maxH, keeping the aspect ratio.
func Fit(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH || w == 0 || h == 0 {
		return img
	}
	scale := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	nw, nh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}
