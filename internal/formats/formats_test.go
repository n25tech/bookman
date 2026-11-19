package formats

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestNaturalSort(t *testing.T) {
	in := []string{"page10.jpg", "Page2.jpg", "page1.jpg", "a/page03.png", "page001.jpg", "cover.jpg"}
	NaturalSort(in)
	want := []string{"a/page03.png", "cover.jpg", "page001.jpg", "page1.jpg", "Page2.jpg", "page10.jpg"}
	if !reflect.DeepEqual(in, want) {
		t.Fatalf("got %v, want %v", in, want)
	}
}

func TestPageFilter(t *testing.T) {
	for name, want := range map[string]bool{
		"01.jpg": true, "x/02.JPEG": true, "03.webp": true, "__MACOSX/01.jpg": false,
		".hidden.png": false, "dir/.thumbs/1.png": false, "ComicInfo.xml": false, "Thumbs.db": false,
	} {
		if got := isPageName(name); got != want {
			t.Errorf("isPageName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTitleFromFilename(t *testing.T) {
	if got := TitleFromFilename("/x/My_Book - Vol 2.pdf"); got != "My Book - Vol 2" {
		t.Fatalf("got %q", got)
	}
}

func pngBytes(t *testing.T, w, h int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCBZ(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.cbz")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	files := map[string][]byte{
		"p10.png":           pngBytes(t, 10, 10, color.White),
		"p2.png":            pngBytes(t, 20, 10, color.Black),
		"p1.png":            pngBytes(t, 800, 1200, color.RGBA{255, 0, 0, 255}),
		"__MACOSX/._p1.png": []byte("junk"),
		"ComicInfo.xml":     []byte("<ComicInfo/>"),
	}
	for name, data := range files {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Modified: time.Now()})
		w.Write(data)
	}
	zw.Close()
	f.Close()

	src, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	c := src.(Comic)
	if !reflect.DeepEqual(c.Pages(), []string{"p1.png", "p2.png", "p10.png"}) {
		t.Fatalf("pages = %v", c.Pages())
	}
	if n, _ := src.PageCount(); n != 3 {
		t.Fatalf("page count %d", n)
	}
	data, ct, err := src.Page(1)
	if err != nil || ct != "image/png" || !bytes.Equal(data, files["p2.png"]) {
		t.Fatalf("page 1: ct=%s err=%v", ct, err)
	}
	cover, err := src.Cover(400, 600)
	if err != nil {
		t.Fatal(err)
	}
	if b := cover.Bounds(); b.Dx() != 400 || b.Dy() != 600 {
		t.Fatalf("cover size %v", b)
	}
	data, _, err = ReadEntry(p, "p10.png")
	if err != nil || !bytes.Equal(data, files["p10.png"]) {
		t.Fatalf("ReadEntry: %v", err)
	}
}

// Real files can't be generated for RAR or committed cheaply, so these run
// against files named in the environment:
//   BOOKMAN_TEST_CBR=/path/x.cbr BOOKMAN_TEST_PDF=/path/y.pdf go test ./internal/formats
func TestRealFiles(t *testing.T) {
	for _, env := range []string{"BOOKMAN_TEST_CBR", "BOOKMAN_TEST_PDF", "BOOKMAN_TEST_CBZ"} {
		p := os.Getenv(env)
		if p == "" {
			continue
		}
		SetupPDF(1, []string{filepath.Dir(p)})
		t.Run(env, func(t *testing.T) {
			start := time.Now()
			src, err := Open(p)
			if err != nil {
				t.Fatal(err)
			}
			defer src.Close()
			n, err := src.PageCount()
			if err != nil || n == 0 {
				t.Fatalf("page count %d, %v", n, err)
			}
			md, _ := src.Metadata()
			img, err := src.Cover(400, 600)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %d pages, %+v, cover %v, %v", filepath.Base(p), n, md, img.Bounds(), time.Since(start))
			if c, ok := src.(Comic); ok {
				last := c.Pages()[n-1]
				var seen int
				data, _, err := ReadRarEntryCaching(p, last, func(string, []byte) { seen++ })
				if err != nil || len(data) == 0 {
					t.Fatalf("reading last page: %v", err)
				}
				t.Logf("last page %s: %d bytes, %d pages cached on the way", last, len(data), seen)
			}
		})
	}
}
