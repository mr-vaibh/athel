ALTER TABLE users ADD COLUMN phone_number TEXT;
UPDATE users SET phone_number = phone WHERE phone_number IS NULL;
