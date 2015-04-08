package main

import (
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

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
	db = createDB(t)
	defer db.Close()

	insertData(t, db, []User{
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
		want    Result
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
		got, gotErr := HandleCard(tc.input)
		if tc.wantErr != nil {
			if gotErr != tc.wantErr {
				t.Errorf("HandleCard(%s) == (%v, %v), want (_, %v)", string(tc.input), got, gotErr, tc.wantErr)
				continue
			}
		}
		if got != tc.want {
			t.Errorf("HandleCard(%s) == (%v, %v), want (%v, %v)", string(tc.input), got, gotErr, tc.want, tc.wantErr)
		}
	}
}

func TestRegistration(t *testing.T) {
	db = createDB(t)
	defer db.Close()

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
		gotUser, gotErr := RegisterUser(tc.name, tc.password)
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
