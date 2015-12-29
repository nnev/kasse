package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
)

func createResponse(req *http.Request, res *httptest.ResponseRecorder) *http.Response {
	return &http.Response{
		Status:           fmt.Sprintf("%d %s", res.Code, http.StatusText(res.Code)),
		StatusCode:       res.Code,
		Proto:            "HTTP/1.1",
		ProtoMajor:       1,
		ProtoMinor:       1,
		Header:           res.HeaderMap,
		Body:             ioutil.NopCloser(res.Body),
		ContentLength:    int64(res.Body.Len()),
		TransferEncoding: nil,
		Close:            false,
		Trailer:          nil,
		Request:          req,
		TLS:              nil,
	}
}

func TestLogin(t *testing.T) {
	k := Kasse{db: createDB(t), log: testLogger(t)}
	k.sessions = sessions.NewCookieStore([]byte("TODO: Set up safer password"))
	h := k.Handler()

	jar, _ := cookiejar.New(nil)

	insertData(t, k.db, []User{
		{
			ID:   1,
			Name: "Merovius",
			// "foobar"
			Password: []byte("$2a$10$HvkgrSxCQxOSFB4vvPd0SuP5urdZUuXSMumMYA5qjli9Mh0pcVDXS"),
		},
		{
			ID:   2,
			Name: "koebi",
			// ""
			Password: []byte("$2a$10$Jt3qpo7xO9DKCbxYNZbFzuRySIB.KSkFnpRo8jv8UYFIng0pOoOlO"),
		},
	}, nil, nil)

	tests := []struct {
		// inputs
		method string
		url    string
		form   url.Values

		// expected outputs
		code    int
		headers map[string]string
		grep    string
	}{
		{"GET", "http://localhost:9000/", nil, http.StatusFound, map[string]string{"Location": "/login.html"}, ""},
		{"GET", "http://localhost:9000/login.html", nil, http.StatusOK, map[string]string{"Content-Type": "text/html"}, "<title>Login</title>"},
		{"POST", "http://localhost:9000/login.html", url.Values{"username": []string{""}, "password": []string{"foobar"}}, http.StatusBadRequest, nil, "Neither username nor password can be empty"},
		{"POST", "http://localhost:9000/login.html", url.Values{"username": []string{"koebi"}, "password": []string{""}}, http.StatusBadRequest, nil, "Neither username nor password can be empty"},
		{"POST", "http://localhost:9000/login.html", url.Values{"username": []string{"koebi"}, "password": []string{"foobar"}}, http.StatusUnauthorized, nil, ""},
		{"POST", "http://localhost:9000/login.html", url.Values{"username": []string{"Merovius"}, "password": []string{"foobaz"}}, http.StatusUnauthorized, nil, ""},
		{"POST", "http://localhost:9000/login.html", url.Values{"username": []string{"Merovius"}, "password": []string{"foobar"}}, http.StatusFound, map[string]string{"Location": "/"}, ""},
		{"GET", "http://localhost:9000/", nil, http.StatusOK, map[string]string{"Content-Type": "text/html"}, "<title>ccchd Kasse</title>"},
	}

	for _, tc := range tests {
		var body io.Reader
		if tc.form != nil {
			body = strings.NewReader(tc.form.Encode())
		}
		req, err := http.NewRequest(tc.method, tc.url, body)
		if err != nil {
			t.Fatalf(`%s %s %v: %v`, tc.method, tc.url, tc.form, err)
		}

		if tc.form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
		}
		for _, c := range jar.Cookies(req.URL) {
			t.Logf("Adding cookie %v", c)
			req.AddCookie(c)
		}

		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)
		if c := rec.Code; c != tc.code {
			t.Fatalf(`%s %s %v has code %d, expected %d`, tc.method, tc.url, tc.form, c, tc.code)
		}

		for k, v := range tc.headers {
			if gv := rec.HeaderMap.Get(k); gv != v {
				t.Fatalf(`%s %s %v has header %q set to %q, expected %q`, tc.method, tc.url, tc.form, k, gv, v)
			}
		}

		if !strings.Contains(rec.Body.String(), tc.grep) {
			t.Fatalf("%s %s %v: Response does not contain %q\nFull Body:\n%s", tc.method, tc.url, tc.form, tc.grep, rec.Body.String())
		}

		res := createResponse(req, rec)
		if c := res.Cookies(); len(c) > 0 {
			t.Logf("Setting cookies %v", res.Cookies())
			jar.SetCookies(req.URL, res.Cookies())
		}
	}
}

func TestNewUser(t *testing.T) {
	k := Kasse{db: createDB(t), log: testLogger(t)}
	k.sessions = sessions.NewCookieStore([]byte("foobar"))
	h := k.Handler()

	jar, _ := cookiejar.New(nil)

	tests := []struct {
		// inputs
		method string
		url    string
		form   url.Values

		// expected outputs
		code    int
		headers map[string]string
		grep    string
	}{
		// test for service being available
		{"GET", "http://localhost:9000/", nil, http.StatusFound, map[string]string{"Location": "/login.html"}, ""},
		// test for login page to be up
		{"GET", "http://localhost:9000/login.html", nil, http.StatusOK, map[string]string{"Content-Type": "text/html"}, "<title>Login</title>"},
		// test for create_user to exist
		{"GET", "http://localhost:9000/create_user.html", nil, http.StatusOK, map[string]string{"Content-Type": "text/html"}, "<title>Create new user</title>"},
		// test for working creation
		{"POST", "http://localhost:9000/create_user.html", url.Values{"username": []string{"foo"}, "password": []string{"bar"}, "confirm": []string{"bar"}}, http.StatusFound, map[string]string{"Location": "/"}, ""},
		// after creation, the user should already exist
		{"POST", "http://localhost:9000/create_user.html", url.Values{"username": []string{"foo"}, "password": []string{"bar"}, "confirm": []string{"bar"}}, http.StatusUnauthorized, nil, "User already exists"},
		// now trying to create user with empty name
		{"POST", "http://localhost:9000/create_user.html", url.Values{"username": []string{""}, "password": []string{"bar"}, "confirm": []string{"bar"}}, http.StatusBadRequest, nil, "Neither username nor password can be empty"},
		// now trying to create user with empty password
		{"POST", "http://localhost:9000/create_user.html", url.Values{"username": []string{"joe"}, "password": []string{""}, "confirm": []string{"bar"}}, http.StatusBadRequest, nil, "Neither username nor password can be empty"},
		// now trying to create user with nonmatching confirmation
		{"POST", "http://localhost:9000/create_user.html", url.Values{"username": []string{"joe"}, "password": []string{"baz"}, "confirm": []string{"bar"}}, http.StatusBadRequest, nil, "Password and confirmation don't match"},
	}

	for _, tc := range tests {
		var body io.Reader
		if tc.form != nil {
			body = strings.NewReader(tc.form.Encode())
		}
		req, err := http.NewRequest(tc.method, tc.url, body)
		if err != nil {
			t.Fatalf(`%s %s %v: %v`, tc.method, tc.url, tc.form, err)
		}

		if tc.form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")
		}
		for _, c := range jar.Cookies(req.URL) {
			t.Logf("Adding cookie %v", c)
			req.AddCookie(c)
		}

		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)
		if c := rec.Code; c != tc.code {
			t.Fatalf(`%s %s %v has code %d, expected %d`, tc.method, tc.url, tc.form, c, tc.code)
		}

		for k, v := range tc.headers {
			if gv := rec.HeaderMap.Get(k); gv != v {
				t.Fatalf(`%s %s %v has header %q set to %q, expected %q`, tc.method, tc.url, tc.form, k, gv, v)
			}
		}

		if !strings.Contains(rec.Body.String(), tc.grep) {
			t.Fatalf("%s %s %v: Response does not contain %q\nFull Body:\n%s", tc.method, tc.url, tc.form, tc.grep, rec.Body.String())
		}

		res := createResponse(req, rec)
		if c := res.Cookies(); len(c) > 0 {
			t.Logf("Setting cookies %v", res.Cookies())
			jar.SetCookies(req.URL, res.Cookies())
		}
	}
}
