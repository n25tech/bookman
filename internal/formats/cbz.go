package formats

import (
	"archive/zip"
	"errors"
	"fmt"
	"image"
	"io"
)

type cbz struct {
	comicBase
	zr    *zip.ReadCloser
	files map[string]*zip.File
}

func openCBZ(path string) (*cbz, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	c := &cbz{zr: zr, files: map[string]*zip.File{}}
	var names []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		c.files[f.Name] = f
		names = append(names, f.Name)
	}
	c.pages = pageList(names)
	return c, nil
}

func (c *cbz) Format() string { return "cbz" }
func (c *cbz) Close() error   { return c.zr.Close() }

func (c *cbz) Page(n int) ([]byte, string, error) {
	if n < 0 || n >= len(c.pages) {
		return nil, "", fmt.Errorf("page %d out of range", n)
	}
	return readZipFile(c.files[c.pages[n]])
}

func (c *cbz) Cover(maxW, maxH int) (image.Image, error) {
	if len(c.pages) == 0 {
		return nil, errors.New("archive has no images")
	}
	data, _, err := c.Page(0)
	if err != nil {
		return nil, err
	}
	return coverFrom(data, maxW, maxH)
}

func readZipFile(f *zip.File) ([]byte, string, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 256<<20))
	return data, ContentType(f.Name), err
}

func readZipEntry(path, entry string) ([]byte, string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == entry {
			return readZipFile(f)
		}
	}
	return nil, "", fmt.Errorf("entry %q not found", entry)
}
