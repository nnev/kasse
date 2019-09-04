package main

import (
	"database/sql"
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Merovius/go-misc/lcd2usb"
	"github.com/gorilla/context"
	"github.com/gorilla/handlers"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var (
	// Defaults for development
	driver   = flag.String("sql-driver", "sqlite3", "The SQL driver to use for the database")
	connect  = flag.String("connect", "kasse.sqlite", "The connection specification for the database")
	listen   = flag.String("listen", "localhost:9000", "Where to listen for HTTP connections")
	hardware = flag.Bool("hardware", true, "Whether hardware is plugged in")
)

func init() {
	gob.Register(User{})
}

// NFCEvent contains an event at the NFC reader. Either UID or Err is nil.
type NFCEvent struct {
	UID []byte
	Err error
}

// Kasse collects all state of the application in a central type, to make
// parallel testing possible.
type Kasse struct {
	db           *sqlx.DB
	log          *log.Logger
	sessions     sessions.Store
	card         (chan []byte)
	registration sync.Mutex
}

// User represents a user in the system (as in the database schema).
type User struct {
	ID       int    `db:"user_id"`
	Name     string `db:"name"`
	Password []byte `db:"password"`
}

// Card represents a card in the system (as in the database schema).
type Card struct {
	ID          []byte `db:"card_id"`
	User        int    `db:"user_id"`
	Description string `db:"description"`
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

// ResultCode is the action taken by a swipe of a card. It should be
// communicated to the user.
type ResultCode int

const (
	_ ResultCode = iota
	// PaymentMade means the charge was applied successfully and there are
	// sufficient funds left in the account.
	PaymentMade
	// LowBalance means the charge was applied successfully, but the account is
	// nearly empty and should be recharged soon.
	LowBalance
	// AccountEmpty means the charge was not applied, because there are not
	// enough funds left in the account.
	AccountEmpty
	// UnknownCard means that the swipe will be ignored
	UnknownCard
)

// Result is the action taken by a swipe of a card. It contains all information
// to be communicated to the user.
type Result struct {
	Code    ResultCode
	UID     []byte
	User    string
	Account float32
}

func flashLCD(lcd *lcd2usb.Device, text string, r, g, b uint8) error {
	lcd.Color(r, g, b)
	for i, l := range strings.Split(text, "\n") {
		if len(l) > 16 {
			l = l[:16]
		}
		if i > 2 {
			break
		}
		lcd.CursorPosition(1, uint8(i+1))
		fmt.Fprint(lcd, l)
	}
	// TODO: Make flag
	time.Sleep(time.Second)
	lcd.Color(0, 0, 255)
	lcd.Clear()
	return nil
}

// Print writes the result to a 16x2 LCD display.
func (res *Result) Print(lcd *lcd2usb.Device) error {
	var r, g, b uint8
	// TODO(mero): Make sure format does not overflow (floating point)
	text := fmt.Sprintf("Card: %x\n%-9s%.2fE", res.UID, res.User, res.Account)
	switch res.Code {
	default:
		r, g, b = 255, 255, 255
	case PaymentMade:
		r, g, b = 0, 255, 0
	case LowBalance:
		r, g, b = 255, 50, 0
	case AccountEmpty:
		r, g, b = 255, 0, 0
	}
	return flashLCD(lcd, text, r, g, b)
}

// String implements fmt.Stringer.
func (r ResultCode) String() string {
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

// ErrCardExists means that an already registered card was tried to register
// again.
var ErrCardExists = errors.New("card already registered")

// ErrWrongAuth means that an invalid username or password was provided for
// Authentication.
var ErrWrongAuth = errors.New("wrong username or password")

// HandleCard handles the swiping of a new card. It looks up the user the card
// belongs to and checks the account balance. It returns PaymentMade, when the
// account has been charged correctly, LowBalance if there is less than 5€ left
// after the charge (the charge is still made) and AccountEmpty when there is
// no balance left on the account. The account is charged if and only if the
// returned error is nil.
func (k *Kasse) HandleCard(uid []byte) (*Result, error) {
	k.log.Printf("Card %x was swiped", uid)

	// if some routine is reading from the card channel, return nil and no error, since all functionality should be handled by the listening routine.
	select {
	case k.card <- uid:
		return &Result{
			Code:    UnknownCard,
			UID:     uid,
			User:    "",
			Account: 0,
		}, nil
	default:
		// do nothing and simply continue with execution
	}

	tx, err := k.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get user this card belongs to
	var user User
	if err := tx.Get(&user, `SELECT users.user_id, name, password FROM cards LEFT JOIN users ON cards.user_id = users.user_id WHERE card_id = $1`, uid); err != nil {
		k.log.Println("Card not found in database")

		return &Result{
			Code:    UnknownCard,
			UID:     uid,
			User:    "",
			Account: 0,
		}, ErrCardNotFound
	}
	k.log.Printf("Card belongs to %v", user.Name)

	// Get account balance of this user
	var balance int64
	var b sql.NullInt64
	if err := tx.Get(&b, `SELECT SUM(amount) FROM transactions WHERE user_id = $1`, user.ID); err != nil {
		k.log.Println("Could not get balance:", err)
		return nil, err
	}
	if b.Valid {
		balance = b.Int64
	}
	k.log.Printf("Account balance is %d", balance)

	res := &Result{
		UID:     uid,
		User:    user.Name,
		Account: float32(balance) / 100,
	}
	if balance < 100 {
		res.Code = AccountEmpty
		return res, nil
	}

	// Insert new transaction
	if _, err := tx.Exec(`INSERT INTO transactions (user_id, card_id, time, amount, kind) VALUES ($1, $2, $3, $4, $5)`, user.ID, uid, time.Now(), -100, "Kartenswipe"); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if balance < 600 {
		k.log.Println("balance is low")
		res.Code = LowBalance
	} else {
		res.Code = PaymentMade
	}
	k.log.Println("returning")
	return res, nil
}

// RegisterUser creates a new row in the user table, with the given username
// and password. It returns a populated User and no error on success. If the
// username is already taken, it returns ErrUserExists.
func (k *Kasse) RegisterUser(name string, password []byte) (*User, error) {
	k.log.Printf("Registering user %s", name)

	tx, err := k.db.Beginx()
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

// AddCard adds a card to the database with a given owner and returns a
// populated card struct. It returns ErrCardExists if a card with the given UID
// already exists.
func (k *Kasse) AddCard(uid []byte, owner *User, description string) (*Card, error) {
	k.log.Printf("Adding card %x for owner %s and description %s", uid, owner.Name, description)

	tx, err := k.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// We need to check first if the card already exists, because the error
	// from an insert can't be checked programatically.
	var card Card
	if err := tx.Get(&card, `SELECT card_id, user_id FROM cards WHERE card_id = $1`, uid); err == nil {
		k.log.Println("Card already exists, current owner", card.User)
		return nil, ErrCardExists
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	if _, err := tx.Exec(`INSERT INTO cards (card_id, user_id, description) VALUES ($1, $2, $3)`, uid, owner.ID, description); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	k.log.Println("Card added successfully")

	card.ID = uid
	card.User = owner.ID

	return &card, nil
}

// RemoveCard removes a card. The function checks, if the requesting user is the card owner and prevents removal otherwise. It takes the UID of the card to remove and returns
func (k *Kasse) RemoveCard(uid []byte, user *User) error {
	k.log.Printf("Removing card %x", uid)

	tx, err := k.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// We need to check first if the card actually belongs to the user, which wants to remove it
	var card Card
	if err := tx.Get(&card, `SELECT card_id, user_id FROM cards WHERE card_id = $1 AND user_id = $2`, uid, user.ID); err == sql.ErrNoRows {
		return ErrCardNotFound
	} else if err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM cards WHERE card_id == $1 AND user_id == $2`, card.ID, user.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	k.log.Println("Card removed successfully")

	return nil
}

// UpdateCard updates the description of a card
func (k *Kasse) UpdateCard(uid []byte, user *User, description string) error {
	k.log.Printf("Updating card %x", uid)

	tx, err := k.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// We need to check first if the card actually belongs to the user, which wants to remove it
	var card Card
	if err := tx.Get(&card, `SELECT card_id, user_id FROM cards WHERE card_id = $1 AND user_id = $2`, uid, user.ID); err == sql.ErrNoRows {
		return ErrCardNotFound
	} else if err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE cards SET description = $1 WHERE card_id == $2 AND user_id == $3`, description, card.ID, user.ID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	k.log.Println("Card updated successfully")

	return nil
}

// Authenticate tries to authenticate a given username/password combination
// against the database. It is guaranteed to take at least 200 Milliseconds. It
// returns ErrWrongAuth, if the user or password was wrong. If no error
// occured, it will return a fully populated User.
func (k *Kasse) Authenticate(username string, password []byte) (*User, error) {
	k.log.Printf("Verifying user %v", username)
	delay := time.After(200 * time.Millisecond)
	defer func() {
		<-delay
	}()

	user := new(User)
	if err := k.db.Get(user, `SELECT user_id, name, password FROM users WHERE name = $1`, username); err == sql.ErrNoRows {
		k.log.Printf("No such user %v", username)
		return nil, ErrWrongAuth
	} else if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword(user.Password, password); err == bcrypt.ErrMismatchedHashAndPassword {
		k.log.Println("Wrong password")
		return nil, ErrWrongAuth
	} else if err != nil {
		return nil, err
	}

	k.log.Printf("Successfully authenticated %v", username)
	return user, nil
}

// GetCards gets all cards for a given user.
func (k *Kasse) GetCards(user User) ([]Card, error) {
	var cards []Card
	if err := k.db.Select(&cards, `SELECT card_id, user_id, description FROM cards WHERE user_id = $1`, user.ID); err != nil {
		return nil, err
	}
	return cards, nil
}

// GetCard gets the cards for a given card uid and user.
func (k *Kasse) GetCard(uid []byte, user User) (*Card, error) {
	var cards []Card
	if err := k.db.Select(&cards, `SELECT card_id, user_id, description FROM cards WHERE card_id = $1 AND user_id = $2`, uid, user.ID); err != nil {
		return nil, err
	}
	return &cards[0], nil
}

// GetBalance gets the current balance for a given user.
func (k *Kasse) GetBalance(user User) (int64, error) {
	var b sql.NullInt64
	if err := k.db.Get(&b, `SELECT SUM(amount) FROM transactions WHERE user_id = $1`, user.ID); err != nil {
		k.log.Println("Could not get balance:", err)
		return 0, err
	}
	return b.Int64, nil
}

// GetTransactions gets the last n transactions for a given user. If n ≤ 0, all
// transactions are returnsed.
func (k *Kasse) GetTransactions(user User, n int) ([]Transaction, error) {
	var transactions []Transaction
	var err error
	if n <= 0 {
		err = k.db.Select(&transactions, `SELECT user_id, card_id, time, amount, kind FROM transactions WHERE user_id = $1 ORDER BY time DESC`, user.ID)
	} else {
		err = k.db.Select(&transactions, `SELECT user_id, card_id, time, amount, kind FROM transactions WHERE user_id = $1 ORDER BY time DESC LIMIT $2`, user.ID, n)
	}
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

func main() {
	flag.Parse()

	k := new(Kasse)
	k.log = log.New(os.Stderr, "", log.LstdFlags)

	if db, err := sqlx.Connect(*driver, *connect); err != nil {
		log.Fatal("Could not open database:", err)
	} else {
		k.db = db
	}
	defer func() {
		if err := k.db.Close(); err != nil {
			log.Println("Error closing database:", err)
		}
	}()

	k.card = make(chan []byte)
	k.registration = sync.Mutex{}
	k.sessions = sessions.NewCookieStore([]byte("TODO: Set up safer password"))
	http.Handle("/", handlers.LoggingHandler(os.Stderr, k.Handler()))

	var lcd *lcd2usb.Device
	if *hardware {
		var err error
		if lcd, err = lcd2usb.Open("/dev/ttyACM0", 2, 16); err != nil {
			log.Fatal(err)
		}
	}

	events := make(chan NFCEvent)

	// We have to wrap the call in a func(), because the go statement evaluates
	// it's arguments in the current goroutine, and the argument to log.Fatal
	// blocks in these cases.
	if *hardware {
		go func() {
			log.Fatal(ConnectAndPollNFCReader("", events))
		}()
	}

	RegisterHTTPReader(k)
	go func() {
		log.Printf("Starting Webserver on http://%s/", *listen)
		log.Fatal(http.ListenAndServe(*listen, context.ClearHandler(http.DefaultServeMux)))
	}()

	for {
		ev := <-events
		if ev.Err != nil {
			log.Println(ev.Err)
			continue
		}

		res, err := k.HandleCard(ev.UID)
		if res != nil {
			res.Print(lcd)
		} else {
			// TODO: Distinguish between user-facing errors and internal errors
			flashLCD(lcd, err.Error(), 255, 0, 0)
		}
	}
}
