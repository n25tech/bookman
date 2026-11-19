package formats

import (
	"errors"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

var (
	pdfPoolOnce sync.Once
	pdfPool     pdfium.Pool
	pdfPoolErr  error
	pdfWorkers  = 1
	pdfRoots    []string
)

// SetupPDF sets how many pdfium instances may run at once and which folders
// the WebAssembly sandbox can read. Call before first use.
//
// By default go-pdfium mounts the whole host filesystem into the sandbox, and
// pdfium's font lookup then reads arbitrary files (and can block forever on a
// FIFO or socket). Only the library folders are mounted instead; pdfium's
// built-in fonts cover the standard PDF fonts.
func SetupPDF(workers int, roots []string) {
	pdfWorkers = max(1, workers)
	pdfRoots = roots
}

func getPool() (pdfium.Pool, error) {
	pdfPoolOnce.Do(func() {
		fsc := wazero.NewFSConfig()
		for _, r := range pdfRoots {
			fsc = fsc.WithReadOnlyDirMount(r, r)
		}
		pdfPool, pdfPoolErr = webassembly.Init(webassembly.Config{MinIdle: 0, MaxIdle: 1, MaxTotal: pdfWorkers, FSConfig: fsc})
	})
	return pdfPool, pdfPoolErr
}

type pdfSource struct {
	f    *os.File
	inst pdfium.Pdfium
	doc  references.FPDF_DOCUMENT
}

func openPDF(path string) (*pdfSource, error) {
	pool, err := getPool()
	if err != nil {
		return nil, fmt.Errorf("starting pdfium: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	inst, err := pool.GetInstance(5 * time.Minute)
	if err != nil {
		f.Close()
		return nil, err
	}
	doc, err := inst.OpenDocument(&requests.OpenDocument{FileReader: f, FileReaderSize: st.Size()})
	if err != nil {
		inst.Close()
		f.Close()
		return nil, err
	}
	return &pdfSource{f: f, inst: inst, doc: doc.Document}, nil
}

func (p *pdfSource) Format() string { return "pdf" }

func (p *pdfSource) Close() error {
	p.inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: p.doc})
	p.inst.Close()
	return p.f.Close()
}

func (p *pdfSource) PageCount() (int, error) {
	r, err := p.inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: p.doc})
	if err != nil {
		return 0, err
	}
	return r.PageCount, nil
}

func (p *pdfSource) Metadata() (Metadata, error) {
	get := func(tag string) string {
		r, err := p.inst.FPDF_GetMetaText(&requests.FPDF_GetMetaText{Document: p.doc, Tag: tag})
		if err != nil {
			return ""
		}
		return strings.TrimSpace(r.Value)
	}
	return Metadata{Title: get("Title"), Author: get("Author")}, nil
}

func (p *pdfSource) Cover(maxW, maxH int) (image.Image, error) {
	r, err := p.inst.RenderPageInPixels(&requests.RenderPageInPixels{
		Page:   requests.Page{ByIndex: &requests.PageByIndex{Document: p.doc, Index: 0}},
		Width:  maxW,
		Height: maxH,
	})
	if err != nil {
		return nil, err
	}
	defer r.Cleanup()
	if r.Result.Image == nil {
		return nil, errors.New("pdfium returned no image")
	}
	// Copy out: the buffer belongs to the wasm instance and is freed by Cleanup.
	src := r.Result.Image
	dst := image.NewRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst, nil
}

func (p *pdfSource) Page(int) ([]byte, string, error) { return nil, "", ErrUnsupported }
