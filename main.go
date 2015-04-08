package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var (
	// Defaults for development
	driver  = flag.String("sql-driver", "sqlite3", "The SQL driver to use for the database")
	connect = flag.String("connect", "kasse.sqlite", "The connection specification for the database")
	db      *sqlx.DB
)

// User represents a user in the system (as in the database schema).
type User struct {
	ID       int    `db:"user_id"`
	Name     string `db:"name"`
	Password []byte `db:"password"`
}

// Card represents a card in the system (as in the database schema).
type Card struct {
	ID   []byte `db:"card_id"`
	User int    `db:"user_id"`
}

// Transaction represents a transaction in the system (as in the database
// schema).
type Transaction struct {
	ID     int       `db:"transaction_id"`
	User   int       `db:"user_id"`
	Card   []byte    `db:"card_id"`
	Time   time.Time `db:"time"`
	Amount int       `db:"amount"`
	Kind   string    `db:"kind"`
}

// Result is the action taken by a swipe of a card. It should be communicated
// to the user.
type Result int

const (
	_ Result = iota
	// PaymentMade means the charge was applied successfully and there are
	// sufficient funds left in the account.
	PaymentMade
	// LowBalance means the charge was applied successfully, but the account is
	// nearly empty and should be recharged soon.
	LowBalance
	// AccountEmpty means the charge was not applied, because there are not
	// enough funds left in the account.
	AccountEmpty
)

// String implements fmt.Stringer.
func (r Result) String() string {
	switch r {
	case PaymentMade:
		return "PaymentMade"
	case LowBalance:
		return "LowBalance"
	case AccountEmpty:
		return "AccountEmpty"
	default:
		return fmt.Sprintf("Result(%d)", r)
	}
}

// ErrAccountEmpty means the charge couldn't be applied because there where
// insufficient funds.
var ErrAccountEmpty = errors.New("account is empty")

// ErrCardNotFound means the charge couldn't be applied becaus the card is not
// registered to any user.
var ErrCardNotFound = errors.New("card not found")

// ErrUserExists means that a duplicate username was tried to register.
var ErrUserExists = errors.New("username already taken")

// HandleCard handles the swiping of a new card. It looks up the user the card
// belongs to and checks the account balance. It returns PaymentMade, when the
// account has been charged correctly, LowBalance if there is less than 5€ left
// after the charge (the charge is still made) and AccountEmpty when there is
// no balance left on the account. The account is charged if and only if the
// returned error is nil.
func HandleCard(uid []byte) (Result, error) {
	log.Printf("Card %x was swiped", uid)

	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Get user this card belongs to
	var user User
	if err := tx.Get(&user, `SELECT users.user_id, name, password FROM cards LEFT JOIN users ON cards.user_id = users.user_id WHERE card_id = $1`, uid); err != nil {
		log.Println("Card not found in database")
		return 0, ErrCardNotFound
	}
	log.Printf("Card belongs to %v", user.Name)

	// Get account balance of this user
	var balance int64
	var b sql.NullInt64
	if err := tx.Get(&b, `SELECT SUM(amount) FROM transactions WHERE user_id = $1`, user.ID); err != nil {
		log.Println("Could not get balance:", err)
		return 0, err
	}
	if b.Valid {
		balance = b.Int64
	}
	log.Printf("Account balance is %d", balance)

	if balance < 100 {
		return AccountEmpty, ErrAccountEmpty
	}

	// Insert new transaction
	if _, err := tx.Exec(`INSERT INTO transactions (user_id, card_id, time, amount, kind) VALUES ($1, $2, $3, $4, $5)`, user.ID, uid, time.Now(), -100, "Kartenswipe"); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	if balance < 600 {
		return LowBalance, nil
	}
	return PaymentMade, nil
}

// RegisterUser creates a new row in the user table, with the given username
// and password. It returns a populated User and no error on success. If the
// username is already taken, it returns ErrUserExists.
func RegisterUser(name string, password []byte) (*User, error) {
	log.Printf("Registering user %s", name)

	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	pwhash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// We need to check first if the username is already taken, because the
	// error from an insert can't be checked programmatically.
	var user User
	if err := tx.Get(&user, `SELECT user_id, name, password FROM users WHERE name = $1`, name); err == nil {
		return nil, ErrUserExists
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	result, err := tx.Exec(`INSERT INTO users (name, password) VALUES ($1, $2)`, name, pwhash)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		if err := tx.Get(&id, `SELECT user_id FROM users WHERE name = $1`, name); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	user.ID = int(id)
	user.Name = name
	user.Password = pwhash
	return &user, nil
}

func main() {
	var err error
	if db, err = sqlx.Connect(*driver, *connect); err != nil {
		log.Fatal("Could not open database:", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Println("Error closing database:", err)
		}
	}()

	r, err := NewNFCReader("")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Println("Error closing reader:", err)
		}
	}()

	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	uids := make(chan []byte)
	go func() {
		for {
			uid, err := r.GetNextUID()
			if err != nil {
				log.Println(err)
				close(sigs)
				return
			}
			uids <- uid
		}
	}()

MainLoop:
	for {
		select {
		case uid := <-uids:
			res, err := HandleCard(uid)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			} else {
				fmt.Println(res)
			}
		case sig := <-sigs:
			if sig != nil {
				log.Printf("Got signal %s, exiting\n", sig)
			}
			break MainLoop
		}
	}

}
