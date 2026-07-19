-- +goose Up
ALTER TABLE posts ADD COLUMN body TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE posts DROP COLUMN body;
