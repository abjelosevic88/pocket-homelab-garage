-- Pocket Money: the transaction a record or fill-up was booked as, and whose
-- Money it is in (the person who booked it, who may not own the vehicle).
ALTER TABLE records ADD COLUMN money_tx_id INTEGER;
ALTER TABLE records ADD COLUMN money_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE fuel    ADD COLUMN money_tx_id INTEGER;
ALTER TABLE fuel    ADD COLUMN money_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL;
