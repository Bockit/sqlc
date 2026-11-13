-- name: LaterSiblingShadowsTable :many
WITH busy AS (SELECT * FROM trackers),
     trackers AS (SELECT id FROM depots)
SELECT * FROM busy;

-- name: NamedLikeTheTableItReads :many
WITH depots(a, b, c) AS (SELECT * FROM depots)
SELECT a, b FROM depots;

-- name: NestedLaterSiblingShadowsTable :many
SELECT *
FROM (
  WITH busy AS (SELECT * FROM trackers),
       trackers AS (SELECT id FROM depots)
  SELECT * FROM busy
) s;

-- name: RecursiveReadsItself :many
WITH RECURSIVE chain AS (
  SELECT id, depot_id FROM trucks
  UNION ALL
  SELECT * FROM chain WHERE chain.id < 0
)
SELECT * FROM chain;
