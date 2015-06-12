package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

// HTTPReader implements the Reader interface by registering handlers under
// /reader/ that can be used to emulate swiping.
type HTTPReader struct {
	k *Kasse
}

func RegisterHTTPReader() (*HTTPReader, error) {
	r := mux.NewRouter()
	r.Methods("GET").Path("/reader/").HandlerFunc(HTTPReader{}.Index)
	r.Methods("POST", "GET").Path("/reader/swipe").HandlerFunc(HTTPReader{}.Swipe)
	http.Handle("/reader/", r)
	return &HTTPReader{}, nil
}

var (
	readerIndexTpl = template.Must(template.New("index.html").Parse(`<!DOCTYPE html>
<html>
	<head>
		<meta charset="UTF-8">
	</head>
	<body>
		<h1>Fake NFC reader für die nnev-Getränkekasse</h1>
		<form action="swipe" method="GET">
			<label for="uid">Emuliere swipe von Karte (id in hex)</label>
			<input type="text" name="uid">
			<ul>
			{{ range . }}
				{{ with printf "%x" .ID }}
				<li><a href="swipe?uid={{ . }}">{{ . }}</a></li>
				{{ end }}
			{{ end }}
			</ul>
		</form>
	</body>
</html>`))
	readerSwipeTpl = template.Must(template.New("swipe").Parse(`<!DOCTYPE html>
<html>
	<head>
		<meta charsef="UTF-8">
		<meta http-equiv="refresh" content="2; /reader">
	</head>
	<body>
		<p>{{ . }}</p>
	</body>
</html>`))
)

func (r HTTPReader) Index(res http.ResponseWriter, req *http.Request) {
	var cards []Card

	if err := r.k.db.Select(&cards, `SELECT * FROM cards`); err != nil {
		log.Println("Could not get cards:", err)
	}

	if err := readerIndexTpl.Execute(res, cards); err != nil {
		log.Println("Error executing template:", err)
		panic(err)
	}
}

func (r HTTPReader) Swipe(res http.ResponseWriter, req *http.Request) {
	var uid []byte
	fmt.Sscanf(req.FormValue("uid"), "%x", &uid)

	if len(uid) == 0 {
		res.WriteHeader(400)
		readerSwipeTpl.Execute(res, "Invalid UID")
		return
	}

	result, err := r.k.HandleCard(uid)
	if err == ErrCardNotFound {
		res.WriteHeader(404)
	} else if err != nil {
		res.WriteHeader(400)
	}
	if err != nil {
		readerSwipeTpl.Execute(res, err)
	} else {
		readerSwipeTpl.Execute(res, result)
	}
}
