INSERT INTO users (user_id, name, password) VALUES (1, 'Merovius', 'password');
INSERT INTO users (user_id, name, password) VALUES (2, 'Koebi', 'password1');

INSERT INTO cards (card_id, user_id) VALUES (x'61616161', 1);
INSERT INTO cards (card_id, user_id) VALUES (x'61616162', 1);
INSERT INTO cards (card_id, user_id) VALUES (x'62616161', 2);
INSERT INTO cards (card_id, user_id) VALUES (x'62616162', 2);

INSERT INTO transactions (user_id, card_id, time, amount, kind) VALUES (1, NULL, '2015-04-06 22:59:03', 1000, 'Aufladung');
INSERT INTO transactions (user_id, card_id, time, amount, kind) VALUES (1, x'61616161', '2015-04-06 23:05:27', -100, 'Kartenswipe');
INSERT INTO transactions (user_id, card_id, time, amount, kind) VALUES (1, x'61616162', '2015-04-06 23:37:23', -100, 'Kartenswipe');
