package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/gorilla/mux"
	"net/http"
	"time"
)

// GetLoginPage renders the login page to the user.
func (k *Kasse) GetLoginPage(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/html")

	if err := ExecuteTemplate(res, TemplateInput{Title: "Login", Body: "login.html"}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", http.StatusInternalServerError)
		return
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
	if err != nil && err != ErrWrongAuth {
		k.log.Println("Error authenticating:", err)
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Internal server error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		k.log.Println("Wrong username or password")
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
	if err := session.Save(req, res); err != nil {
		k.log.Printf("Error saving session: %v", err)
	}

	http.Redirect(res, req, redirect, http.StatusFound)
}

// GetNewUserPage renders the page to create a new user.
func (k *Kasse) GetNewUserPage(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/html")

	if err := ExecuteTemplate(res, TemplateInput{Title: "Create new user", Body: "newUser.html"}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", http.StatusInternalServerError)
		return
	}
}

// PostNewUserPage receives a POST request with username and password and tries
// to create a new user. It will redirect to the first Flashvalue in the
// session on success, or to / if none is set and save the authenticated user
// in the session.
func (k *Kasse) PostNewUserPage(res http.ResponseWriter, req *http.Request) {
	username := req.FormValue("username")
	password := []byte(req.FormValue("password"))
	confirm := []byte(req.FormValue("confirm"))

	if username == "" || len(password) == 0 || len(confirm) == 0 {
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Neither username nor password can be empty", http.StatusBadRequest)
		return
	}

	if !bytes.Equal(password, confirm) {
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Password and confirmation don't match", http.StatusBadRequest)
		return
	}

	user, err := k.RegisterUser(username, password)
	if err != nil && err != ErrUserExists {
		k.log.Printf("Registering user %q failed:%v", username, err)
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err == ErrUserExists {
		k.log.Println(err)
		// TODO: Write own Error function, that uses a template for better
		// looking error pages. Also, redirect.
		http.Error(res, "User already exists.", http.StatusForbidden)
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
	if err := session.Save(req, res); err != nil {
		k.log.Printf("Error saving session: %v", err)
	}

	http.Redirect(res, req, redirect, http.StatusFound)
}

// GetDashboard renders a basic dashboard, containing the most important
// information and actions for an account.
func (k *Kasse) GetDashboard(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	ui, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	user := ui.(User)

	cards, err := k.GetCards(user)
	if err != nil {
		k.log.Printf("Could not get cards for user %q: %v", user.Name, err)
		http.Error(res, "Internal error", 500)
		return
	}

	balance, err := k.GetBalance(user)
	if err != nil {
		k.log.Printf("Could not get balance for user %q: %v", user.Name, err)
		http.Error(res, "Internal error", 500)
		return
	}

	transactions, err := k.GetTransactions(user, 5)
	if err != nil {
		k.log.Printf("Could not get transactions for user %q: %v", user.Name, err)
		http.Error(res, "Internal error", 500)
		return
	}

	res.Header().Set("Content-Type", "text/html")

	data := struct {
		User         User
		Balance      float32
		Cards        []Card
		Transactions []Transaction
	}{
		User:         user,
		Balance:      float32(balance) / 100,
		Cards:        cards,
		Transactions: transactions,
	}

	if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "dashboard.html", Data: data}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", 500)
		return
	}
}

// GetLogout logs out the user immediately and redirect to the login page.
func (k *Kasse) GetLogout(res http.ResponseWriter, req *http.Request) {
	defer http.Redirect(res, req, "/login.html", 302)

	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		return
	}

	delete(session.Values, "user")
	if err := session.Save(req, res); err != nil {
		k.log.Printf("Error saving session: %v", err)
	}
}

// GetAddCard renders the add card dialog. The rendered template contains an instruction for the browser to connect to AddCardEvent and listen for the card UID on the next swipe
func (k *Kasse) GetAddCard(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	ui, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}

	user := ui.(User)
	data := struct {
		User        *User
		Description string
		Message     string
	}{
		User:        &user,
		Description: "",
		Message:     "",
	}

	if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "add_card.html", Data: &data}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", 500)
		return
	}
}

// AddCardEvent returns a json containing the next swiped card UID. The UID is obtained using a channel which is written by the HandleCard method
func (k *Kasse) AddCardEvent(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	_, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}

	res.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	res.WriteHeader(http.StatusOK)

	// Only one go routine can listen on the next card swipe. Tell the client, when it obtains the lock
	k.registration.Lock()
	defer k.registration.Unlock()
	if _, err := res.Write([]byte("event: lock\ndata: lock\n\n")); err != nil {
		k.log.Println("Could not write: ", err)
	}
	if f, ok := res.(http.Flusher); ok {
		f.Flush()
	}

	k.log.Println("Waiting for Card")

	// Read from the channel for one minute. If the timeout is exceeded and the registration window is still open on the client, the browser reconnects anyway
	var uid []byte
	ctx, cancel := context.WithTimeout(req.Context(), 1*time.Minute)
	defer cancel()
	select {
	case uid = <-k.card:
	case <-ctx.Done():
		http.Error(res, ctx.Err().Error(), http.StatusRequestTimeout)
		return
	}

	// Send card UID in hexadecimal to client
	uidString := hex.EncodeToString(uid)
	k.log.Println("Card UID obtained! Card uid is", uidString)

	if _, err := res.Write([]byte("event: card\ndata: " + uidString + "\n\n")); err != nil {
		k.log.Println("Could not write: ", err)
	}

	if f, ok := res.(http.Flusher); ok {
		f.Flush()
	}
}

// PostAddCard creates a new Card for the POSTing user
func (k *Kasse) PostAddCard(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	ui, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}

	user := ui.(User)

	err = req.ParseForm()
	if err != nil {
		http.Error(res, "Internal error", http.StatusBadRequest)
	}
	description := req.Form.Get("description")
	uidString := req.Form.Get("uid")

	renderError := func(message string) {
		data := struct {
			User        *User
			Description string
			Message     string
		}{
			User:        &user,
			Description: description,
			Message:     message,
		}

		if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "add_card.html", Data: &data}); err != nil {
			k.log.Println("Could not render template:", err)
			http.Error(res, "Internal error", 500)
			return
		}
	}

	if len(uidString) == 0 {
		renderError("Please swipe Card to register")
		return
	}

	uid, err := hex.DecodeString(uidString)
	if err != nil {
		renderError("Hexadecimal UID could not be decoded")
		return
	}

	_, err = k.AddCard(uid, &user, description)
	if err != nil {
		if err == ErrCardExists {
			renderError("Card is already registered")
		} else {
			renderError("Card could not be added")
		}
		return
	}

	http.Redirect(res, req, "/", 302)
}

// PostRemoveCard removes a card for the POSTing user
func (k *Kasse) PostRemoveCard(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	ui, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}

	user := ui.(User)

	err = req.ParseForm()
	if err != nil {
		http.Error(res, "Internal error", http.StatusBadRequest)
	}
	uidString := req.Form.Get("uid")

	uid, err := hex.DecodeString(uidString)
	if err != nil {
		http.Redirect(res, req, "/", http.StatusNotFound)
		return
	}

	card, err := k.GetCard(uid, user)
	if err != nil {
		http.Redirect(res, req, "/", http.StatusNotFound)
		return
	}

	err = k.RemoveCard(uid, &user)
	if err != nil {
		data := struct {
			Card    *Card
			Message string
		}{
			Card:    card,
			Message: err.Error(),
		}

		if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "edit_card.html", Data: &data}); err != nil {
			k.log.Println("Could not render template:", err)
			http.Error(res, "Internal error", 500)
			return
		}
	} else {
		http.Redirect(res, req, "/", http.StatusFound)
		return
	}

	http.Redirect(res, req, "/", 302)
}

// PostEditCard renders an edit dialog for a given card
func (k *Kasse) PostEditCard(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	ui, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}

	user := ui.(User)

	uids, ok := req.URL.Query()["uid"]
	if !ok || len(uids[0]) < 1 {
		http.Error(res, "No uid given", http.StatusBadRequest)
		return
	}
	uidString := uids[0]

	uid, err := hex.DecodeString(uidString)
	if err != nil {
		http.Redirect(res, req, "/", http.StatusNotFound)
		return
	}

	card, err := k.GetCard(uid, user)
	if err != nil {
		http.Redirect(res, req, "/", http.StatusNotFound)
		return
	}

	data := struct {
		Card    *Card
		Message string
	}{
		Card:    card,
		Message: "",
	}

	if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "edit_card.html", Data: &data}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", 500)
		return
	}
}

// PostUpdateCard saves card changes and redirects to the dashboard
func (k *Kasse) PostUpdateCard(res http.ResponseWriter, req *http.Request) {
	session, err := k.sessions.Get(req, "nnev-kasse")
	if err != nil {
		http.Redirect(res, req, "/login.html", 302)
		return
	}
	ui, ok := session.Values["user"]
	if !ok {
		http.Redirect(res, req, "/login.html", 302)
		return
	}

	user := ui.(User)

	err = req.ParseForm()
	if err != nil {
		http.Error(res, "Internal error", http.StatusBadRequest)
	}
	uidString := req.Form.Get("uid")

	uid, err := hex.DecodeString(uidString)
	if err != nil {
		http.Redirect(res, req, "/", http.StatusNotFound)
	}

	description := req.Form.Get("description")

	err = k.UpdateCard(uid, &user, description)
	if err != nil {
		card, err2 := k.GetCard(uid, user)
		message := err.Error()
		if err2 != nil {
			message = err2.Error()
		}

		data := struct {
			Card    *Card
			Message string
		}{
			Card:    card,
			Message: message,
		}

		if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "edit_card.html", Data: &data}); err != nil {
			k.log.Println("Could not render template:", err)
			http.Error(res, "Internal error", 500)
			return
		}
	}

	http.Redirect(res, req, "/", http.StatusFound)
	return
}

// Handler returns a http.Handler for the webinterface.
func (k *Kasse) Handler() http.Handler {
	r := mux.NewRouter()
	r.Methods("GET").Path("/").HandlerFunc(k.GetDashboard)
	r.Methods("GET").PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.Methods("GET").Path("/login.html").HandlerFunc(k.GetLoginPage)
	r.Methods("POST").Path("/login.html").HandlerFunc(k.PostLoginPage)
	r.Methods("GET").Path("/logout.html").HandlerFunc(k.GetLogout)
	r.Methods("GET").Path("/create_user.html").HandlerFunc(k.GetNewUserPage)
	r.Methods("POST").Path("/create_user.html").HandlerFunc(k.PostNewUserPage)
	r.Methods("GET").Path("/add_card.html").HandlerFunc(k.GetAddCard)
	r.Methods("POST").Path("/add_card.html").HandlerFunc(k.PostAddCard)
	r.Methods("GET").Path("/add_card_event").HandlerFunc(k.AddCardEvent)
	r.Methods("POST").Path("/remove_card.html").HandlerFunc(k.PostRemoveCard)
	r.Methods("GET").Path("/edit_card.html").HandlerFunc(k.PostEditCard)
	r.Methods("POST").Path("/update_card.html").HandlerFunc(k.PostUpdateCard)
	return r
}
