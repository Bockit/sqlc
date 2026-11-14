-- name: GetAuthor :one
SELECT * FROM authors
WHERE id = $1 LIMIT 1;

-- name: ListAuthors :many
SELECT * FROM authors
ORDER BY name;

-- name: CreateAuthor :one
INSERT INTO authors (
          name, bio
) VALUES (
  $1, $2
)
RETURNING *;

-- name: DeleteAuthor :exec
DELETE FROM authors
WHERE id = $1;

-- name: CountAuthorsThroughCTE :one
WITH counted AS (
  SELECT count(*) AS authors, max(name) AS last_name FROM authors
)
SELECT authors, last_name FROM counted;

-- name: LongestBioThroughSubquery :one
SELECT * FROM (SELECT max(length(bio)) AS longest FROM authors) x;

-- name: LongestBioOrZero :one
SELECT COALESCE(x.longest, 0) AS longest FROM (SELECT max(length(bio)) AS longest FROM authors) x;
