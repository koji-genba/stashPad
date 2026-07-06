ALTER TABLE works ADD COLUMN rating INTEGER CHECK (rating IS NULL OR (rating BETWEEN 1 AND 5));

CREATE INDEX IF NOT EXISTS idx_works_hidden_rating ON works(hidden, rating);
