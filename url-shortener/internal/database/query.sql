-- name: GetLongUrl :one
SELECT long_url FROM urls
WHERE short_url = $1;

-- name: PutShortUrl :one
WITH inserted AS (
  INSERT INTO urls (short_url, long_url) VALUES ($1, $2)
  ON CONFLICT (short_url) DO NOTHING
  RETURNING long_url
)
SELECT long_url FROM inserted
UNION ALL
SELECT long_url FROM urls
WHERE short_url = $1
LIMIT 1;

-- name: CleanOldUrls :exec
DELETE FROM urls
WHERE created_at < $1;

