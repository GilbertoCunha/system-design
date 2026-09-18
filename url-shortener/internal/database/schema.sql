CREATE table IF NOT EXISTS urls (
  id BIGSERIAL PRIMARY KEY,
  short_url CHAR(32) NOT NULL,
  long_url TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS short_url
ON urls(short_url);
