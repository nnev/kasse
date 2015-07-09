package main

import (
	"bytes"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type testWriter struct {
	*testing.T
}

func (t testWriter) Write(b []byte) (n int, err error) {
	t.Log(string(b))
	return len(b), nil
}

func testLogger(t *testing.T) *log.Logger {
	return log.New(testWriter{t}, "", 0)
}

type TestReader []struct {
	UID []byte
	Err error
}

func (t *TestReader) GetNextUID() ([]byte, error) {
	if len(*t) == 0 {
		return nil, errors.New("no uids left")
	}
	h := (*t)[0]
	*t = (*t)[1:]
	return h.UID, h.Err
}

func (t *TestReader) Close() error {
	return nil
}

func createDB(t *testing.T) *sqlx.DB {
	db, err := sqlx.Connect("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("could not create in-memory database: %v", err)
	}
	_, err = sqlx.LoadFile(db, "schema.sql")
	if err != nil {
		t.Fatalf("could not load schema: %v", err)
	}
	return db
}

func insertData(t *testing.T, db *sqlx.DB, us []User, cs []Card, ts []Transaction) {
	for _, v := range us {
		_, err := db.Exec("INSERT INTO users (user_id, name, password) VALUES ($1, $2, $3)", v.ID, v.Name, v.Password)
		if err != nil {
			t.Fatalf("could not insert user %v: %v", v, err)
		}
	}
	for _, v := range cs {
		_, err := db.Exec("INSERT INTO cards (card_id, user_id) VALUES ($1, $2)", v.ID, v.User)
		if err != nil {
			t.Fatalf("could not insert card %v: %v", v, err)
		}
	}
	for _, v := range ts {
		_, err := db.Exec("INSERT INTO transactions (transaction_id, user_id, card_id, time, amount, kind) VALUES ($1, $2, $3, $4, $5, $6)", v.ID, v.User, v.Card, v.Time, v.Amount, v.Kind)
		if err != nil {
			t.Fatalf("could not insert transaction %v: %v", v, err)
		}
	}
}

func TestHandleCard(t *testing.T) {
	t.Parallel()

	k := Kasse{db: createDB(t), log: testLogger(t)}
	defer k.db.Close()

	insertData(t, k.db, []User{
		{ID: 1, Name: "Merovius", Password: []byte("password")},
		{ID: 2, Name: "Koebi", Password: []byte("password1")},
	}, []Card{
		{ID: []byte("aaaa"), User: 1},
		{ID: []byte("aaab"), User: 1},
		{ID: []byte("baaa"), User: 2},
		{ID: []byte("baab"), User: 2},
	}, []Transaction{
		{ID: 1, User: 1, Card: nil, Time: time.Date(2015, 04, 06, 22, 59, 03, 0, time.FixedZone("TST", 3600)), Amount: 1000, Kind: "Aufladung"},
		{ID: 2, User: 1, Card: []byte("aaaa"), Time: time.Date(2015, 04, 06, 23, 05, 27, 0, time.FixedZone("TST", 3600)), Amount: -100, Kind: "Kartenswipe"},
		{ID: 3, User: 1, Card: []byte("aaab"), Time: time.Date(2015, 04, 06, 22, 59, 03, 0, time.FixedZone("TST", 3600)), Amount: -100, Kind: "Kartenswipe"},
	})

	tcs := []struct {
		input   []byte
		wantErr error
		want    ResultCode
	}{
		{[]byte("foobar"), ErrCardNotFound, 0},
		{[]byte("baaa"), ErrAccountEmpty, AccountEmpty},
		{[]byte("baab"), ErrAccountEmpty, AccountEmpty},
		{[]byte("aaaa"), nil, PaymentMade},
		{[]byte("aaab"), nil, PaymentMade},
		{[]byte("aaaa"), nil, PaymentMade},
		{[]byte("aaab"), nil, LowBalance},
		{[]byte("aaab"), nil, LowBalance},
		{[]byte("aaaa"), nil, LowBalance},
		{[]byte("aaab"), nil, LowBalance},
		{[]byte("aaaa"), nil, LowBalance},
		{[]byte("aaab"), ErrAccountEmpty, AccountEmpty},
		{[]byte("aaaa"), ErrAccountEmpty, AccountEmpty},
	}

	for _, tc := range tcs {
		got, gotErr := k.HandleCard(tc.input)
		if tc.wantErr != nil {
			if gotErr != tc.wantErr {
				t.Errorf("HandleCard(%s) == (%v, %v), want (_, %v)", string(tc.input), got, gotErr, tc.wantErr)
			}
			continue
		}
		if got == nil || got.Code != tc.want {
			t.Errorf("HandleCard(%s) == (%v, %v), want (%v, %v)", string(tc.input), got, gotErr, tc.want, tc.wantErr)
		}
	}
}

func TestGetTransactions(t *testing.T) {
	t.Parallel()

	k := Kasse{db: createDB(t), log: testLogger(t)}
	defer k.db.Close()

	mero := User{ID: 1, Name: "Merovius", Password: []byte("password")}
	koebi := User{ID: 2, Name: "Koebi", Password: []byte("password1")}

	insertData(t, k.db, []User{
		mero,
		koebi,
	}, []Card{
		{ID: []byte("aaaa"), User: 1},
		{ID: []byte("aaab"), User: 1},
		{ID: []byte("baaa"), User: 2},
		{ID: []byte("baab"), User: 2},
	}, []Transaction{
		{ID: 1, User: 1, Card: nil, Time: time.Date(2015, 4, 6, 22, 59, 3, 0, time.FixedZone("TST", 3600)), Amount: 1000, Kind: "Aufladung"},
		{ID: 2, User: 1, Card: []byte("aaaa"), Time: time.Date(2015, 4, 6, 23, 5, 27, 0, time.FixedZone("TST", 3600)), Amount: -100, Kind: "Kartenswipe"},
		{ID: 3, User: 1, Card: []byte("aaab"), Time: time.Date(2015, 4, 6, 23, 7, 23, 0, time.FixedZone("TST", 3600)), Amount: -100, Kind: "Kartenswipe"},
	})

	tcs := []struct {
		user    User
		n       int
		wantErr error
		amounts []int
	}{
		{koebi, 0, nil, nil},
		{koebi, 23, nil, nil},
		{mero, 2, nil, []int{-100, -100}},
		{mero, 0, nil, []int{-100, -100, 1000}},
	}

	for _, tc := range tcs {
		got, gotErr := k.GetTransactions(tc.user, tc.n)
		if tc.wantErr != nil {
			if gotErr != tc.wantErr {
				t.Errorf("GetTransactions(%v, %v) == (%v, %v), want (_, %v)", tc.user.Name, tc.n, got, gotErr, tc.wantErr)
			}
			continue
		} else if gotErr != nil {
			t.Errorf("GetTransactions(%v, %v) == (%v, %v), want (_, %v)", tc.user.Name, tc.n, got, gotErr, nil)
			continue
		}

		if len(got) != len(tc.amounts) {
			t.Errorf("GetTransactions(%v, %v) == (%v, %v), want %v", tc.user.Name, tc.n, got, gotErr, tc.amounts)
			continue
		}

		for i := range got {
			if got[i].Amount != tc.amounts[i] {
				t.Errorf("GetTransactions(%v, %v) == (%v, %v), want %v", tc.user.Name, tc.n, got, gotErr, tc.amounts)
				continue
			}
		}
	}
}

func TestRegistration(t *testing.T) {
	t.Parallel()

	k := Kasse{db: createDB(t), log: testLogger(t)}
	defer k.db.Close()

	tcs := []struct {
		name     string
		password []byte
		wantUser bool
		wantErr  error
	}{
		{"Merovius", []byte("foobar"), true, nil},
		{"Koebi", []byte("password"), true, nil},
		{"Merovius", []byte("password1"), false, ErrUserExists},
	}

	for _, tc := range tcs {
		gotUser, gotErr := k.RegisterUser(tc.name, tc.password)
		if gotErr != tc.wantErr {
			t.Errorf("RegisterUser(%s, %q) == (%v, %v), want (_, %v)", tc.name, tc.password, gotUser, gotErr, tc.wantErr)
			continue
		}

		if !tc.wantUser {
			if gotUser != nil {
				t.Errorf("RegisterUser(%s, %q) == (%v, %v), want (nil, %v)", tc.name, tc.password, gotUser, gotErr, tc.wantErr)
			}
			continue
		}

		if err := bcrypt.CompareHashAndPassword(gotUser.Password, tc.password); err != nil {
			t.Errorf("bcrypt.CompareHashAndPassword(%q, %s) = %v, want nil", gotUser.Password, tc.password, err)
		}
	}
}

func TestAddCard(t *testing.T) {
	t.Parallel()

	k := Kasse{db: createDB(t), log: testLogger(t)}
	defer k.db.Close()

	mero := &User{ID: 1, Name: "Merovius", Password: []byte("password")}
	koebi := &User{ID: 2, Name: "Koebi", Password: []byte("password1")}

	insertData(t, k.db, []User{*mero, *koebi}, nil, nil)

	tcs := []struct {
		uid      []byte
		user     *User
		wantCard bool
		wantErr  error
	}{
		{[]byte("aaaa"), mero, true, nil},
		{[]byte("aaab"), mero, true, nil},
		{[]byte("baaa"), koebi, true, nil},
		{[]byte("baab"), koebi, true, nil},
		{[]byte("aaaa"), mero, false, ErrCardExists},
		{[]byte("aaaa"), koebi, false, ErrCardExists},
	}

	for _, tc := range tcs {
		gotCard, gotErr := k.AddCard(tc.uid, tc.user)
		if gotErr != tc.wantErr {
			t.Errorf("AddCard(%x, %v) == (%v, %v), want (_, %v)", tc.uid, tc.user, gotCard, gotErr, tc.wantErr)
			continue
		}

		if !tc.wantCard {
			if gotCard != nil {
				t.Errorf("AddCard(%x, %v) == (%v, %v), want (nil, %v)", tc.uid, tc.user, gotCard, gotErr, tc.wantErr)
			}
			continue
		}

		wantCard := Card{
			ID:   tc.uid,
			User: tc.user.ID,
		}

		if bytes.Compare(gotCard.ID, wantCard.ID) != 0 || gotCard.User != wantCard.User {
			t.Errorf("AddCard(%x, %v) == (%v, %v), want (%v, %v)", tc.uid, tc.user, gotCard, gotErr, wantCard, tc.wantErr)
		}
	}
}

func usersAreEqual(a, b *User) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	if a.ID != b.ID {
		return false
	}
	if a.Name != b.Name {
		return false
	}
	if bytes.Compare(a.Password, b.Password) != 0 {
		return false
	}
	return true
}

func TestAuthentication(t *testing.T) {
	t.Parallel()

	k := Kasse{db: createDB(t), log: testLogger(t)}
	defer k.db.Close()

	// bcrypt hash of "foobar"
	mero := User{
		ID:       1,
		Name:     "Merovius",
		Password: []byte("$2a$10$HvkgrSxCQxOSFB4vvPd0SuP5urdZUuXSMumMYA5qjli9Mh0pcVDXS"),
	}
	insertData(t, k.db, []User{mero}, nil, nil)

	tcs := []struct {
		username string
		password []byte
		wantUser *User
		wantErr  error
	}{
		{"Merovius", []byte("foobar"), &mero, nil},
		{"Merovius", []byte("wrong password"), nil, ErrWrongAuth},
		{"Koebi", []byte("somepassword"), nil, ErrWrongAuth},
	}

	for _, tc := range tcs {
		before := time.Now()
		gotUser, gotErr := k.Authenticate(tc.username, tc.password)
		after := time.Now()
		if after.Sub(before) <= 200*time.Millisecond {
			t.Errorf(`Authenticate took %v, should be at least 200ms`, after.Sub(before))
		}

		if gotErr != tc.wantErr || !usersAreEqual(gotUser, tc.wantUser) {
			t.Fatalf(`Authenticate(%v, %v) == (%v, %v), expected (%v, %v)`, tc.username, tc.password, gotUser, gotErr, tc.wantUser, tc.wantErr)
		}
	}
}

// TODO: Test the HTTP handlers
