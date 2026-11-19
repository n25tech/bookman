package formats

import (
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/nwaples/rardecode/v2"
)

type cbr struct {
	comicBase
	path string
}

func openCBR(path string) (*cbr, error) {
	files, err := rardecode.List(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		if f.IsDir || f.Encrypted {
			continue
		}
		names = append(names, f.Name)
	}
	return &cbr{comicBase: comicBase{pages: pageList(names)}, path: path}, nil
}

func (c *cbr) Format() string { return "cbr" }
func (c *cbr) Close() error   { return nil }

func (c *cbr) Page(n int) ([]byte, string, error) {
	if n < 0 || n >= len(c.pages) {
		return nil, "", fmt.Errorf("page %d out of range", n)
	}
	return readRarEntry(c.path, c.pages[n], nil)
}

func (c *cbr) Cover(maxW, maxH int) (image.Image, error) {
	if len(c.pages) == 0 {
		return nil, errors.New("archive has no images")
	}
	data, _, err := c.Page(0)
	if err != nil {
		return nil, err
	}
	return coverFrom(data, maxW, maxH)
}

// readRarEntry extracts one entry. Non-solid entries are opened directly. Solid
// archives must be decoded in order, so it streams from the start; every page
// image passed on the way is handed to seen (if non-nil) so a cache can keep it.
func readRarEntry(path, entry string, seen func(name string, data []byte)) ([]byte, string, error) {
	files, err := rardecode.List(path)
	if err != nil {
		return nil, "", err
	}
	for _, f := range files {
		if f.Name != entry {
			continue
		}
		if !f.Solid {
			rc, err := f.Open()
			if err == nil {
				defer rc.Close()
				data, err := io.ReadAll(io.LimitReader(rc, 256<<20))
				return data, ContentType(entry), err
			}
			if !errors.Is(err, rardecode.ErrSolidOpen) {
				return nil, "", err
			}
		}
		break
	}

	rc, err := rardecode.OpenReader(path)
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	for {
		h, err := rc.Next()
		if err == io.EOF {
			return nil, "", fmt.Errorf("entry %q not found", entry)
		}
		if err != nil {
			return nil, "", err
		}
		if h.IsDir {
			continue
		}
		if h.Name == entry {
			data, err := io.ReadAll(io.LimitReader(rc, 256<<20))
			return data, ContentType(entry), err
		}
		if seen != nil && isPageName(h.Name) {
			if data, err := io.ReadAll(io.LimitReader(rc, 256<<20)); err == nil {
				seen(h.Name, data)
			}
		}
	}
}

// ReadRarEntryCaching reads a comic entry; for solid RAR archives it also hands
// every page decoded on the way to seen. Zip containers are read directly.
func ReadRarEntryCaching(path, entry string, seen func(name string, data []byte)) ([]byte, string, error) {
	if sniffArchive(path) == "rar" {
		return readRarEntry(path, entry, seen)
	}
	return readZipEntry(path, entry)
}
