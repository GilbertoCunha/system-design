-- name: GetLongUrl :one
SELECT long_url FROM urls
WHERE short_url = $1;

-- name: PutShortUrl :exec
INSERT INTO urls (
  short_url, long_url
) VALUES (
  $1, $2
);

-- name: CleanOldUrls :exec
DELETE FROM urls
WHERE created_at < $1;

