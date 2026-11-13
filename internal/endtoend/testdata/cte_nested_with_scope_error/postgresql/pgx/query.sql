-- name: OuterNamesInner :many
SELECT d.id
FROM (WITH d AS (SELECT id FROM depots) SELECT d.id FROM d) x, d;
