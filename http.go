package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

// GetLoginPage renders the login page to the user.
func (k *Kasse) GetLoginPage(res http.ResponseWriter, req *http.Request) {
	if err := ExecuteTemplate(res, TemplateInput{Title: "Login", Body: "login.html"}); err != nil {
		log.Println("Could not render template:", err)
	}
}

// PostLoginPage receives a POST request with username and password and tries
// to authenticate the user. It will redirect to the first Flashvalue in the
// session on success, or to / if none is set and save the authenticated user
// in the session.
func (k *Kasse) PostLoginPage(res http.ResponseWriter, req *http.Request) {
	username := req.FormValue("username")
	password := []byte(req.FormValue("password"))

	if username == "" || len(password) == 0 {
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Neither username nor password can be empty", http.StatusBadRequest)
		return
	}

	user, err := k.Authenticate(username, password)
	if err != nil {
		log.Println("Error authenticating:", err)
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Internal server error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		log.Println("Wrong username or password")
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Wrong username or password", http.StatusUnauthorized)
		return
	}

	session, _ := k.sessions.Get(req, "nnev-kasse")
	redirect := "/"
	if v := session.Flashes(); len(v) > 0 {
		if s, ok := v[0].(string); ok {
			redirect = s
		}
	}
	session.Values["user"] = user
	session.Save(req, res)

	http.Redirect(res, req, redirect, http.StatusFound)
}

// RegisterSessionHandler registers all handlers necessary for authentication
// and session handling.
func (k *Kasse) RegisterHandlers() {
	r := mux.NewRouter()
	r.Methods("GET").Path("/login.html").HandlerFunc(k.GetLoginPage)
	r.Methods("POST").Path("/login.html").HandlerFunc(k.PostLoginPage)
	http.Handle("/", r)
}
