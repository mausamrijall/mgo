-- +goose Up
CREATE TABLE posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL
);

-- +goose Down
DROP TABLE posts;
