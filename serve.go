package main

import (
	"camlistore.org/pkg/osutil"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"path"
	"strconv"
	"sync"
	"time"
)

var serveMutex sync.Mutex

func serve() {
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/content2", content2Handler)
	http.HandleFunc("/content", contentHandler)
	http.HandleFunc("/url", urlHandler)
	http.HandleFunc("/additional/", additionalHandler)
	http.HandleFunc("/", indexHandler)

	nl, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ":" + strconv.Itoa(nl.Addr().(*net.TCPAddr).Port)
	fmt.Printf("Listening on: %s\n", port)

	s := &http.Server{
		Handler:        http.DefaultServeMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		time.Sleep(1 * time.Second)
		osutil.OpenURL("http://127.0.0.1" + port + "/")
	}()

	must(s.Serve(nl))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	serveMutex.Lock()
	defer serveMutex.Unlock()

	urls := listUrls()
	must(indexPage.Execute(w, urls))
}

var indexPage = template.Must(template.New("indexPage").Parse(`
<html>
	<head>
		<title>Index</title>
	</head>
	<body>
		<form action="search" method="get">
		Search: <input name="q" type="text" value=""/>
		</form>
		<table>
			<th>
				<tr>
					<td>id</td>
					<td>important</td>
					<td>extract</td>
					<td>url</td>
				</tr>
			</th>
			{{range .}}
			<tr>
				<td><a href="url?id={{.Id}}">{{.Id}}</a></td>
				<td><a href="content2?id={{.Id}}">extract</a></td>
				<td><a href="{{.Url}}">source</a></td>
				<td><a href="url?id={{.Id}}">{{.Title}}</a><br>{{.Url}}</td>
			</tr>
			{{end}}
		</table>
	</body>
</html>
`))

func urlHandler(w http.ResponseWriter, r *http.Request) {
	serveMutex.Lock()
	defer serveMutex.Unlock()

	idstr, ok := r.URL.Query()["id"]
	if !ok || (len(idstr) < 1) {
		indexHandler(w, r)
	}
	id, err := strconv.Atoi(idstr[0])
	if err != nil {
		indexHandler(w, r)
	}
	url, ok := getUrl(id)
	if !ok {
		indexHandler(w, r)
	}
	urlRevisions := url.listUrlRevisions()
	if len(urlRevisions) == 1 {
		w.Header().Add("Location", fmt.Sprintf("content?id=%d&retrieved_date=%d", id, urlRevisions[0].RetrievedDate))
		w.WriteHeader(302)
	} else {
		must(urlPage.Execute(w, map[string]interface{}{"url": url, "revs": urlRevisions}))
	}
}

var urlPage = template.Must(template.New("urlPage").Parse(`
<html>
	<head>
		<title>{{.url.Url}}</title>
	</head>
	<body>
		<p>Url id {{.url.Id}}<p>
		<p><a href="{{.url.Url}}">{{.url.Url}}</a></p>
		<p><a href="content2?id={{.url.Id}}">Last Extracted Text</a></p>
		<table>
			<th>
				<tr>
					<td>Retrieved Date</td>
					<td>IsGz</td>
					<td>IsDiff</td>
					<td>Size</td>
				</tr>
			</th>
			{{$id := .url.Id}}
			{{range .revs}}
			<tr>
				<td><a href="content?id={{$id}}&retrieved_date={{.RetrievedDate}}">{{.RetrievedDate}}</a></td>
				<td>{{.IsGz}}</td>
				<td>{{.IsDiff}}</td>
				<td>{{.Size}}</td>
			</tr>
			{{end}}
		</table>
	</body>
</html>
`))

func contentHandler(w http.ResponseWriter, r *http.Request) {
	serveMutex.Lock()
	defer serveMutex.Unlock()

	idstr, ok := r.URL.Query()["id"]
	if !ok || (len(idstr) < 1) {
		indexHandler(w, r)
	}
	id, err := strconv.Atoi(idstr[0])
	if err != nil {
		indexHandler(w, r)
	}
	url, ok := getUrl(id)
	if !ok {
		indexHandler(w, r)
	}
	retrievedDateStr, ok := r.URL.Query()["retrieved_date"]
	if !ok || (len(retrievedDateStr) < 1) {
		urlHandler(w, r)
	}
	retrievedDate, err := strconv.Atoi(retrievedDateStr[0])
	if err != nil {
		indexHandler(w, r)
	}
	content, _, ok := url.GetContent(retrievedDate)
	w.Header().Add("Content-Type", "text/html")
	w.Write(content)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	serveMutex.Lock()
	defer serveMutex.Unlock()

	qs, ok := r.URL.Query()["q"]
	q := ""
	if ok {
		q = qs[0]
	}

	results := []Result{}
	if ok {
		results = search(q)
	}

	must(serPage.Execute(w, map[string]interface{}{"q": q, "results": results}))
}

var serPage = template.Must(template.New("serPage").Parse(`
<html>
	<head>
		<title>Search results: {{.q}}</title>
	</head>
	<body>
		<p><form action="search" method="get">
		Query: <input name="q" type="text" value="{{.q}}"/>
		</form></p>
		<table>
			<th>
				<tr>
					<td>Id</td>
					<td>Title</td>
				</tr>
			</th>
			{{range .results}}
			<tr>
				<td>{{.UrlId}}</td>
				<td><a href="url?id={{.UrlId}}">{{.Title}}</a></td>
			</tr>
			{{end}}
		</table>
	</body>
</html>
`))

func content2Handler(w http.ResponseWriter, r *http.Request) {
	serveMutex.Lock()
	defer serveMutex.Unlock()

	idstr, ok := r.URL.Query()["id"]
	if !ok || (len(idstr) < 1) {
		indexHandler(w, r)
	}
	id, err := strconv.Atoi(idstr[0])
	if err != nil {
		indexHandler(w, r)
	}
	url, ok := getUrl(id)
	if !ok {
		indexHandler(w, r)
	}
	title, text, _ := url.GetContent2()
	must(content2Page.Execute(w, map[string]interface{}{"url": url, "title": title, "text": text}))
}

var content2Page = template.Must(template.New("content2Page").Parse(`
<html>
	<head>
		<title>Extraction for {{.title}}</title>
	</head>
	<body>
		<p><a href="url?id={{.url.Id}}">Url page</a></p>
		<h1>{{.title}}</h1>
		<p>
		{{.text}}
		</p>
	</body>
</html>
`))

func additionalHandler(w http.ResponseWriter, r *http.Request) {
	serveMutex.Lock()
	defer serveMutex.Unlock()

	name := path.Base(r.URL.Path)
	contentType, content, ok := GetContentAddressable(name)
	if !ok {
		w.WriteHeader(404)
		return
	}

	w.Header().Add("Content-Type", contentType)
	w.WriteHeader(200)

	w.Write(content)
}
