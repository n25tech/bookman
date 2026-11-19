package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/n25tech/bookman/internal/config"
	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
	"github.com/n25tech/bookman/internal/library"
	"github.com/n25tech/bookman/internal/thumbs"
)

// The integration test needs an empty PostgreSQL database it may wipe:
//
//	BOOKMAN_TEST_DATABASE_URL=postgres://user:pw@localhost:5432/bookman_test go test ./internal/api
func testDB(t *testing.T) *db.DB {
	dsn := os.Getenv("BOOKMAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BOOKMAN_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d, err = db.Open(ctx, dsn) // re-run migrations on the clean schema
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func writeCBZ(t *testing.T, path string, pages int) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for i := 1; i <= pages; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 60, 90))
		for p := range img.Pix {
			img.Pix[p] = byte(i * 40)
		}
		img.Set(0, 0, color.Black)
		w, _ := zw.Create(filepath.Join("pages", strings.Repeat("0", 2)+string(rune('0'+i))+".png"))
		png.Encode(w, img)
	}
	zw.Close()
	f.Close()
}

// writeCP437CBZ writes a zip whose entry names are CP437 bytes (no UTF-8 flag),
// as old Windows zip tools do.
func writeCP437CBZ(t *testing.T, path string) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"caf\x82 01.png", "caf\x82 02.png"} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, NonUTF8: true, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 4, 6))
		png.Encode(w, img)
	}
	zw.Close()
	f.Close()
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *client) do(method, path string, body any, out any, headers ...string) int {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestAPI(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	root := t.TempDir()
	writeCBZ(t, filepath.Join(root, "Series A", "Issue 2.cbz"), 3)
	writeCBZ(t, filepath.Join(root, "Series A", "Issue 10.cbz"), 4)
	writeCBZ(t, filepath.Join(root, "Excluded", "skip.cbz"), 2)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not a book"), 0o644)
	writeCP437CBZ(t, filepath.Join(root, "Old", "cp437.cbz"))

	covers := thumbs.New(d, 1)
	sc := library.NewScanner(d, []config.Library{{Name: "Comics", Path: root, Exclude: []string{"Excluded"}}}, covers)
	if err := sc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	sc.ScanAll(ctx)
	if st := sc.Status(); st.Added != 3 || st.Errors != 0 {
		t.Fatalf("scan status %+v", st)
	}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go covers.Run(cctx)

	srv := &Server{DB: d, Scanner: sc, Thumbs: covers, Pages: formats.NewPageCache(1 << 20),
		Web: fstest.MapFS{"index.html": {Data: []byte("<html>app</html>")}}, ScanAll: func() { sc.ScanAll(ctx) }}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}

	// First run: setup required, then the admin exists and setup is closed.
	var st map[string]bool
	c.do("GET", "/api/auth/status", nil, &st)
	if !st["setup_required"] {
		t.Fatal("setup should be required")
	}
	if code := c.do("GET", "/api/libraries", nil, nil); code != 401 {
		t.Fatalf("unauthenticated libraries: %d", code)
	}
	if code := c.do("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "short"}, nil); code != 400 {
		t.Fatalf("weak password accepted: %d", code)
	}
	if code := c.do("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "password123"}, nil); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	if code := c.do("POST", "/api/auth/setup", map[string]string{"username": "evil", "password": "password123"}, nil); code != 409 {
		t.Fatalf("second setup: %d", code)
	}

	var libs []db.Library
	c.do("GET", "/api/libraries", nil, &libs)
	if len(libs) != 1 || libs[0].BookCount != 3 {
		t.Fatalf("libraries %+v", libs)
	}
	// The folder browser mirrors the layout on disk.
	var br struct {
		Folders []Folder
		Books   []db.Book
	}
	c.do("GET", "/api/libraries/"+itoa(libs[0].ID)+"/browse", nil, &br)
	var seriesA *Folder
	for i, f := range br.Folders {
		if f.Name == "Excluded" {
			t.Fatal("an excluded folder is listed")
		}
		if f.Name == "Series A" {
			seriesA = &br.Folders[i]
		}
	}
	if seriesA == nil || seriesA.BookCount != 2 || seriesA.Path != "Series A" || len(br.Books) != 0 {
		t.Fatalf("browse root: %+v", br)
	}
	c.do("GET", "/api/libraries/"+itoa(libs[0].ID)+"/browse?path=Series%20A", nil, &br)
	if len(br.Folders) != 0 || len(br.Books) != 2 || br.Books[0].Title != "Issue 2" {
		t.Fatalf("browse Series A: %+v", br)
	}
	if code := c.do("GET", "/api/libraries/"+itoa(libs[0].ID)+"/browse?path=../../etc", nil, nil); code != 404 {
		t.Fatalf("browse outside the library: %d", code)
	}

	var series []db.Series
	c.do("GET", "/api/libraries/"+itoa(libs[0].ID)+"/series", nil, &series)
	if len(series) != 2 || series[1].Name != "Series A" {
		t.Fatalf("series %+v", series)
	}
	// Pages with non-UTF-8 entry names are stored and served.
	var old struct{ Books []db.Book }
	c.do("GET", "/api/series/"+itoa(series[0].ID), nil, &old)
	if len(old.Books) != 1 || old.Books[0].PageCount != 2 {
		t.Fatalf("cp437 book: %+v", old.Books)
	}
	for _, n := range []string{"1", "2"} {
		res, _ := c.http.Get(ts.URL + "/api/books/" + itoa(old.Books[0].ID) + "/pages/" + n)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("cp437 page %s: %d", n, res.StatusCode)
		}
	}
	series = series[1:]
	var sv struct{ Books []db.Book }
	c.do("GET", "/api/series/"+itoa(series[0].ID), nil, &sv)
	if len(sv.Books) != 2 || sv.Books[0].Title != "Issue 2" || sv.Books[1].Title != "Issue 10" {
		t.Fatalf("natural order: %+v", sv.Books)
	}
	book := sv.Books[1] // 4 pages
	bid := itoa(book.ID)

	// Pages and the raw file (with Range).
	res, _ := c.http.Get(ts.URL + "/api/books/" + bid + "/pages/4")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("page: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	res.Body.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/api/books/"+bid+"/file", nil)
	req.Header.Set("Range", "bytes=0-9")
	res, _ = c.http.Do(req)
	if res.StatusCode != 206 {
		t.Fatalf("range: %d", res.StatusCode)
	}
	res.Body.Close()

	// Progress: last write wins by client_ts.
	now := time.Now().UnixMilli()
	var p db.Progress
	c.do("PUT", "/api/books/"+bid+"/progress", map[string]any{"page": 3, "offset": 0, "client_ts": now, "device": "A"}, &p)
	if p.Page != 3 || p.Device != "A" {
		t.Fatalf("first save %+v", p)
	}
	c.do("PUT", "/api/books/"+bid+"/progress", map[string]any{"page": 1, "offset": 0, "client_ts": now - 1000, "device": "B"}, &p)
	if p.Page != 3 || p.Device != "A" {
		t.Fatalf("stale save won: %+v", p)
	}
	// Beacon-style: text/plain POST with a same-origin Origin header.
	beacon := `{"page":4,"offset":0.95,"client_ts":` + itoa(now+1000) + `,"device":"B"}`
	req, _ = http.NewRequest("POST", ts.URL+"/api/books/"+bid+"/progress", strings.NewReader(beacon))
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Origin", ts.URL)
	res, _ = c.http.Do(req)
	json.NewDecoder(res.Body).Decode(&p)
	res.Body.Close()
	if res.StatusCode != 200 || p.Page != 4 || !p.Finished {
		t.Fatalf("beacon save: %d %+v", res.StatusCode, p)
	}
	// Cross-site requests are refused.
	if code := c.do("PUT", "/api/books/"+bid+"/progress", map[string]any{"page": 1, "client_ts": now + 5000, "device": "X"}, nil, "Origin", "https://evil.example"); code != 403 {
		t.Fatalf("cross-origin save: %d", code)
	}

	// Sessions: device A has one, device B has one.
	var sess []db.ReadingSession
	c.do("GET", "/api/books/"+bid+"/sessions", nil, &sess)
	if len(sess) != 2 {
		t.Fatalf("sessions %+v", sess)
	}

	// Finished books leave "continue reading"; unfinished ones show up.
	var cont []db.Book
	c.do("GET", "/api/continue-reading", nil, &cont)
	if len(cont) != 0 {
		t.Fatalf("finished book in continue reading: %+v", cont)
	}
	c.do("POST", "/api/books/"+bid+"/finished", map[string]bool{"finished": false}, &p)
	c.do("GET", "/api/continue-reading", nil, &cont)
	if len(cont) != 1 || cont[0].ID != book.ID {
		t.Fatalf("continue reading %+v", cont)
	}

	// Notes: create, edit, soft delete, restore, export.
	var n db.Note
	if code := c.do("POST", "/api/books/"+bid+"/notes", map[string]any{"page": 2, "body_md": "**hello**"}, &n); code != 201 {
		t.Fatalf("create note: %d", code)
	}
	if code := c.do("POST", "/api/books/"+bid+"/notes", map[string]any{"page": 2, "quote": "x", "anchor_start": 0, "anchor_end": 1}, nil); code != 400 {
		t.Fatalf("anchored note on a comic: %d", code)
	}
	body := "edited"
	c.do("PATCH", "/api/notes/"+itoa(n.ID), map[string]any{"body_md": body}, &n)
	if n.BodyMD != "edited" {
		t.Fatalf("patch: %+v", n)
	}
	c.do("DELETE", "/api/notes/"+itoa(n.ID), nil, nil)
	var notes []db.Note
	c.do("GET", "/api/books/"+bid+"/notes", nil, &notes)
	if len(notes) != 0 {
		t.Fatal("deleted note listed")
	}
	c.do("PATCH", "/api/notes/"+itoa(n.ID), map[string]any{"deleted": false}, nil)
	c.do("GET", "/api/books/"+bid+"/notes", nil, &notes)
	if len(notes) != 1 {
		t.Fatal("undo failed")
	}
	res, _ = c.http.Get(ts.URL + "/api/books/" + bid + "/notes/export.md")
	var md bytes.Buffer
	md.ReadFrom(res.Body)
	res.Body.Close()
	if !strings.Contains(md.String(), "## Page 2") || !strings.Contains(md.String(), "edited") {
		t.Fatalf("export: %s", md.String())
	}

	// A moved file keeps its id (and so its progress and notes); a deleted one is marked missing.
	if err := os.Rename(filepath.Join(root, "Series A", "Issue 10.cbz"), filepath.Join(root, "Issue 10 moved.cbz")); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(root, "Series A", "Issue 2.cbz"))
	sc.ScanAll(ctx)
	var bd struct{ Book db.Book }
	c.do("GET", "/api/books/"+bid, nil, &bd)
	if bd.Book.Status != "ok" || bd.Book.RelPath != "Issue 10 moved.cbz" || bd.Book.Progress == nil {
		t.Fatalf("after move: %+v", bd.Book)
	}
	c.do("GET", "/api/books/"+itoa(sv.Books[0].ID), nil, &bd)
	if bd.Book.Status != "missing" {
		t.Fatalf("deleted file status %q", bd.Book.Status)
	}

	// Covers are generated in the background.
	deadline := time.Now().Add(10 * time.Second)
	for {
		res, _ = c.http.Get(ts.URL + "/api/books/" + bid + "/cover")
		res.Body.Close()
		if res.StatusCode == 200 || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if res.StatusCode != 200 || res.Header.Get("ETag") == "" {
		t.Fatalf("cover: %d", res.StatusCode)
	}

	// The SPA fallback serves index.html for client routes; unknown API paths are JSON 404s.
	res, _ = c.http.Get(ts.URL + "/library/1")
	if res.StatusCode != 200 {
		t.Fatalf("spa: %d", res.StatusCode)
	}
	res.Body.Close()
	var e map[string]string
	if code := c.do("GET", "/api/nope", nil, &e); code != 404 || e["error"] == "" {
		t.Fatalf("api 404: %d %v", code, e)
	}

	// Logout ends the session.
	c.do("POST", "/api/auth/logout", nil, nil)
	if code := c.do("GET", "/api/auth/me", nil, nil); code != 401 {
		t.Fatalf("after logout: %d", code)
	}
}

func itoa(n int64) string {
	return strings.TrimSpace(strings.Replace(string(must(json.Marshal(n))), `"`, "", -1))
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
