CREATE TABLE users (
	-- users contains all user-data. An entry in this table corresponds to one
	-- specific person. The account balance is reconstructed completely out of the
	-- transactions table, to reduce duplication of information.


	-- user_id is a sequential identifier.
	user_id INTEGER NOT NULL,
	-- name is the username used for display and login.
	name TEXT,
	-- password is a bcrypt-hashed password.
	password BINARY,

	-- constraints
	PRIMARY KEY (user_id)
);

CREATE TABLE cards (
	-- cards contains all card-data. An entry in this table corresponds to one
	-- physical card. Every user can have an arbitrary number of cards.


	-- card_id is a sequential identifier.
	card_id BINARY NOT NULL,
	-- user_id is the user this card belongs to.
	user_id INTEGER,

	-- constraints
	PRIMARY KEY (card_id),
	FOREIGN KEY (user_id) REFERENCES users(user_id)
);

CREATE TABLE transactions (
	-- transactions contains all transactions.


	-- transaction_id is a sequential identifier.
	--transaction_id INTEGER NOT NULL,
	-- user_id is the user that made this transaction.
	user_id INTEGER,
	-- card_id is the card this transaction was made with, if any.
	card_id INTEGER,
	-- time is the server-time this transaction happened.
	time DATETIME,
	-- amount is the (potentially negative) amount (in cents) of this
	-- transaction.
	amount INTEGER,
	-- kind describes how this transaction was made: via touching an nfc tag to
	-- the reader or by manually adding an amount in the web-interface.
	kind TEXT,

	-- constraints
	--PRIMARY KEY (transaction_id),
	FOREIGN KEY (user_id) REFERENCES users(user_id),
	FOREIGN KEY (card_id) REFERENCES cards(card_id)
);
