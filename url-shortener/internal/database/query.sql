-- name: GetLongUrl :one
SELECT long_url FROM urls
WHERE short_url = $1;

-- name: PutShortUrl :one
INSERT INTO urls (short_url, long_url) VALUES ($1, $2)
ON CONFLICT (short_url) DO UPDATE SET long_url = urls.long_url
RETURNING long_url;

-- name: CleanOldUrls :exec
DELETE FROM urls
WHERE created_at < $1;

