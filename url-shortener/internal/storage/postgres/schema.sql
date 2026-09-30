CREATE table IF NOT EXISTS urls (
  short_url TEXT PRIMARY KEY,
  long_url TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
