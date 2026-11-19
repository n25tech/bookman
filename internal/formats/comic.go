package formats

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// comicBase holds what CBZ and CBR share: the page list and cover decoding.
type comicBase struct {
	pages []string
}

func (c *comicBase) Pages() []string             { return c.pages }
func (c *comicBase) PageCount() (int, error)     { return len(c.pages), nil }
func (c *comicBase) Metadata() (Metadata, error) { return Metadata{}, nil }

func pageList(names []string) []string {
	var pages []string
	for _, n := range names {
		if isPageName(n) {
			pages = append(pages, n)
		}
	}
	NaturalSort(pages)
	return pages
}

func coverFrom(data []byte, maxW, maxH int) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return Fit(img, maxW, maxH), nil
}
