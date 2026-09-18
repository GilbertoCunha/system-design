-- +goose Up
CREATE table IF NOT EXISTS urls (
  id BIGSERIAL PRIMARY KEY,
  short_url CHAR(32) UNIQUE NOT NULL,
  long_url TEXT UNIQUE NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_urls_short_url
ON urls(short_url);

-- +goose Down
DROP INDEX IF EXISTS idx_urls_short_url;
DROP TABLE IF EXISTS urls;
