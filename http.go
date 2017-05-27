package main

import (
	"bytes"
	"net/http"

	"github.com/gorilla/mux"
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

// GetAddCard redirects to a new form helping the user to add a card.
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
		User *User
		UID  []byte
	}{
		User: &user,
		UID:  []byte{},
	}

	k.user = &user

	if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "add_card.html", Data: data}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", 500)
		return
	}

	data.UID = <-k.cards

	if err := ExecuteTemplate(res, TemplateInput{Title: "ccchd Kasse", Body: "add_card.html", Data: data}); err != nil {
		k.log.Println("Could not render template:", err)
		http.Error(res, "Internal error", 500)
		return
	}
}

func (k *Kasse) PostAddCard(res http.ResponseWriter, req *http.Request) {

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
	return r
}
