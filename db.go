package main

import (
	"bytes"
	"code.google.com/p/gosqlite/sqlite"
	"compress/gzip"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"github.com/kr/binarydist"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var dbConn *sqlite.Conn

type Url struct {
	Id          int
	Url         string
	IsImportant bool
	LastVisit   int
	IsNew       bool
	Title       string
}

type Revision struct {
	RetrievedDate int
	IsGz, IsDiff  bool
	Size          int
}

func hasTable(name string) bool {
	stmt, err := dbConn.Prepare("SELECT name FROM sqlite_master WHERE name = ?")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec(name))
	return stmt.Next()
}

func createDatabase() (err error) {
	err = dbConn.Exec(`CREATE TABLE IF NOT EXISTS urls (
		id integer primary key autoincrement not null,
		url text not null,
		important boolean not null,
		last_visit date not null
	)`)
	if err != nil {
		return
	}
	err = dbConn.Exec(`CREATE TABLE IF NOT EXISTS content (
		url_id integer not null,
		isdiff boolean not null,
		isgz boolean not null,
		retrieved date not null,
		content blob not null
	)`)
	if err != nil {
		return
	}

	err = dbConn.Exec(`CREATE TABLE IF NOT EXISTS additional (
		contentid text primary key not null,
		url text not null,
		contenttype text not null,
		content blob not null
	)`)

	if !hasTable("content2idx") {
		err = dbConn.Exec(`CREATE VIRTUAL TABLE content2idx USING fts3(
			url_id integer primary key autoincrement not null, 
			title text,
			ttext text
		)`)
		if err != nil {
			return
		}
	}

	return
}

// Compresses c if the compressed version is going to be significantly shorter
func maybeCompress(c []byte) (cc []byte, isgz bool) {
	bw := bytes.NewBuffer([]byte{})
	gw, _ := gzip.NewWriterLevel(bw, gzip.BestCompression)
	gw.Write(c)
	gw.Close()
	bs := bw.Bytes()

	if len(bs) < int(float32(len(c))*MINGAIN) {
		return bs, true
	} else {
		return []byte(c), false
	}
}

func uncompress(a []byte) []byte {
	r, err := gzip.NewReader(bytes.NewReader(a))
	must(err)
	defer r.Close()
	x, err := ioutil.ReadAll(r)
	must(err)
	return x
}

// Creates a diff from o to c then compresses it. Both operations are done only if the result is significantly shorter.
// The function will just return c uncompressed if it's the shortest solution
func maybeDiffCompress(c, o []byte) (cc []byte, isdiff bool, isgz bool) {
	if debugProcessing {
		fmt.Printf("\tContent is %d/%d bytes, diffing\n", len(o), len(c))
	}
	patch := bytes.NewBuffer([]byte{})
	binarydist.Diff(bytes.NewReader(o), bytes.NewReader(c), patch)
	db := patch.Bytes()
	if len(db) < int(float32(len(c))*MINGAIN) {
		if debugProcessing {
			fmt.Printf("\tCompressing diff\n")
		}
		dbc, isgz := maybeCompress(db)
		if debugProcessing {
			fmt.Printf("\tDone\n")
		}
		return dbc, true, isgz
	} else {
		if debugProcessing {
			fmt.Printf("\tCompressing original\n")
		}
		cc, isgz := maybeCompress([]byte(c))
		if debugProcessing {
			fmt.Printf("\tDone\n")
		}
		return cc, false, isgz
	}
}

func patch(base []byte, changes []byte) []byte {
	new := bytes.NewBuffer([]byte{})
	binarydist.Patch(bytes.NewReader(base), new, bytes.NewReader(changes))
	return new.Bytes()
}

func decodeChanges(enc []byte) []DecoratedChange {
	encr := bytes.NewReader(enc)
	n, err := binary.ReadVarint(encr)
	must(err)
	r := make([]DecoratedChange, n)
	for i := range r {
		var x int64
		x, err = binary.ReadVarint(encr)
		must(err)
		r[i].A = int(x)
		x, err = binary.ReadVarint(encr)
		must(err)
		r[i].Del = int(x)
		x, err = binary.ReadVarint(encr)
		must(err)
		r[i].Ins = int(x)
		r[i].InsText = make([]byte, r[i].Ins)
		_, err = io.ReadFull(encr, r[i].InsText)
		must(err)
	}
	return r
}

func writeVarint(w io.Writer, x int, buf []byte) {
	n := binary.PutVarint(buf, int64(x))
	w.Write(buf[:n])
}

// Returns most recent stored version of the url's content and the number of diffs since the last full storage
func (u *Url) GetContent(atDate int) ([]byte, int, bool) {
	var stmt *sqlite.Stmt
	var err error

	if atDate < 0 {
		stmt, err = dbConn.Prepare("select isgz, retrieved, content  from content where url_id = ? and isdiff = 0 order by retrieved desc limit 1")
	} else {
		stmt, err = dbConn.Prepare("select isgz, retrieved, content  from content where url_id = ? and isdiff = 0 and retrieved <= ? order by retrieved desc limit 1")
	}
	must(err)
	defer stmt.Finalize()
	if atDate < 0 {
		must(stmt.Exec(u.Id))
	} else {
		must(stmt.Exec(u.Id, atDate))
	}
	if !stmt.Next() {
		return []byte{}, 0, false
	}

	var isgz bool
	var retrievedDate int
	var baseContent []byte

	stmt.Scan(&isgz, &retrievedDate, &baseContent)

	if isgz {
		baseContent = uncompress(baseContent)
	}

	n := 0
	var stmt2 *sqlite.Stmt

	if atDate < 0 {
		stmt2, err = dbConn.Prepare("select isgz, content from content where url_id = ? and isdiff = 1 and retrieved > ? order by retrieved asc")
	} else {
		stmt2, err = dbConn.Prepare("select isgz, content from content where url_id = ? and isdiff = 1 and retrieved > ? and retrieved <= ? order by retrieved asc")
	}

	must(err)
	defer stmt2.Finalize()
	if atDate < 0 {
		must(stmt2.Exec(u.Id, retrievedDate))
	} else {
		must(stmt2.Exec(u.Id, retrievedDate, atDate))
	}
	for stmt.Next() {
		var content []byte
		stmt2.Scan(&isgz, &content)
		if isgz {
			content = uncompress(content)
		}
		baseContent = patch(baseContent, content)
		n++
	}

	return baseContent, n, true
}

// Stores a new version of the content for u. if newRecord is true the new version will be inserted, otherwise we will just update the (single) record for the url
func (u *Url) StoreContent(cc []byte, isdiff, isgz, newRecord bool) {
	if newRecord {
		must(dbConn.Exec("insert into content (url_id, isdiff, isgz, retrieved, content) values (?, ?, ?, ?, ?)", u.Id, isdiff, isgz, time.Now().Unix(), cc))
	} else {
		must(dbConn.Exec("update content set isdiff = ?, isgz = ?, retrieved = ?, content = ? where url_id = ?", isdiff, isgz, time.Now().Unix(), cc, u.Id))
	}
}

func (u *Url) StoreContent2(title, text string) {
	must(dbConn.Exec("insert or replace into content2idx (url_id, title, ttext) values (?, ?, ?)", u.Id, title, text))
}

func (u *Url) GetContent2() (title string, text string, ok bool) {
	stmt, err := dbConn.Prepare("select title, ttext from content2idx where url_id = ?")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec(u.Id))
	if !stmt.Next() {
		ok = false
		return
	}
	must(stmt.Scan(&title, &text))
	ok = true
	return
}

// Gets informations pertaining url if present, otherwise adds it to the database
func Lookup(url string, important bool, lastVisit int, recur bool) (r Url) {
	r.Url = url
	stmt, err := dbConn.Prepare("select id, important, last_visit from urls where url = ?")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec(url))
	if stmt.Next() {
		r.IsNew = false
		must(stmt.Scan(&r.Id, &r.IsImportant, &r.LastVisit))

		if r.IsImportant {
			// Don't have to update last visit or to change the value for the important field
			return
		}

		r.IsImportant = important
		r.LastVisit = lastVisit

		must(dbConn.Exec("update urls set important = ?, last_visit = ? where id = ?", r.IsImportant, r.LastVisit, r.Id))
		return
	} else {
		if !recur {
			fmt.Fprintf(os.Stderr, "Could not insert url %s in database\n", url)
			os.Exit(1)
		}
		must(dbConn.Exec("insert into urls(url, important, last_visit) values (?, ?, ?)", url, important, lastVisit))
		r := Lookup(url, important, lastVisit, false)
		r.IsNew = true
		return r
	}
}

func listUrls() []Url {
	stmt, err := dbConn.Prepare("select id, url, important, last_visit, min(title) from urls, content2idx where urls.id = content2idx.url_id group by urls.id")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec())
	r := make([]Url, 0)
	for stmt.Next() {
		var url Url
		must(stmt.Scan(&url.Id, &url.Url, &url.IsImportant, &url.LastVisit, &url.Title))
		r = append(r, url)
	}
	return r
}

func getUrl(id int) (r Url, ok bool) {
	stmt, err := dbConn.Prepare("select url, important, last_visit from urls where id = ?")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec(id))
	if !stmt.Next() {
		ok = false
		return
	}
	r.Id = id
	must(stmt.Scan(&r.Url, &r.IsImportant, &r.LastVisit))
	ok = true
	return
}

func (u *Url) listUrlRevisions() (r []Revision) {
	stmt, err := dbConn.Prepare("select retrieved, isgz, isdiff, content from content where url_id = ?")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec(u.Id))
	r = []Revision{}
	for stmt.Next() {
		var rev Revision
		var content []byte
		must(stmt.Scan(&rev.RetrievedDate, &rev.IsGz, &rev.IsDiff, &content))
		rev.Size = len(content)
		r = append(r, rev)
	}
	return
}

func (u *Url) Remove() {
	must(dbConn.Exec("delete from urls where id = ?", u.Id))
}

type Result struct {
	UrlId string
	Url   string
	Title string
}

func search(q string) []Result {
	stmt, err := dbConn.Prepare("select url_id, title from content2idx where title match ? union select url_id, title from content2idx where ttext match ?")
	must(err)
	defer stmt.Finalize()
	must(stmt.Exec(q, q))
	r := []Result{}
	for stmt.Next() {
		var a Result
		must(stmt.Scan(&a.UrlId, &a.Title))
		r = append(r, a)
	}

	for i := range r {
		id, _ := strconv.Atoi(r[i].UrlId)
		u, ok := getUrl(id)
		if ok {
			r[i].Url = u.Url
		}
		//TODO
	}
	return r
}

func RetrieveContentAddressable(url string, dbAccessLock sync.Mutex) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	contentId := contentAddressableId(content)

	dbAccessLock.Lock()
	defer dbAccessLock.Unlock()
	must(dbConn.Exec("insert or ignore into additional(contentid, url, contenttype, content) values (?, ?, ?, ?)", contentId, url, contentType, content))

	return contentId, nil
}

func GetContentAddressable(name string) (contentType string, content []byte, ok bool) {
	stmt, err := dbConn.Prepare("select contenttype, content from additional where contentid = ?")
	must(err)
	defer stmt.Finalize()
	stmt.Exec(name)
	if !stmt.Next() {
		return "", nil, false
	}

	ok = true
	must(stmt.Scan(&contentType, &content))
	v := make([]byte, len(content))
	copy(v, content)
	content = v
	return
}

var HEX_DIGITS = []byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'}

func contentAddressableId(content []byte) string {
	b := sha1.Sum(content)
	r := make([]byte, len(b)*2)

	for i := range b {
		r[i*2] = HEX_DIGITS[b[i]>>4]
		r[i*2+1] = HEX_DIGITS[b[i]&0x0f]
	}
	return string(r)
}
