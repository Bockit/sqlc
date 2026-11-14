-- name: CorrelatedRecursive :many
SELECT d.id, (
  WITH RECURSIVE up(id, depth) AS (
    SELECT d.id, 0
    UNION ALL
    SELECT up.id, up.depth + 1 FROM up WHERE up.depth < 3
  )
  SELECT max(up.depth)::int FROM up
) AS deepest
FROM depots d
WHERE d.name = @name;

-- name: CorrelatedUnderStar :many
SELECT *
FROM (
  SELECT d.id, (WITH u AS (SELECT d.name AS label) SELECT u.label FROM u) AS label
  FROM depots d
  WHERE d.name <> @skip
) x;

-- name: CorrelatedInCTE :many
WITH named AS (
  SELECT t.id, (WITH u AS (SELECT t.plate AS plate) SELECT u.plate FROM u) AS plate
  FROM trucks t
  WHERE t.depot_id = @depot_id
)
SELECT * FROM named;

-- name: CorrelatedTwoLevels :many
SELECT d.id, (
  SELECT count(*)
  FROM trucks t
  WHERE t.depot_id = d.id
    AND EXISTS (WITH u AS (SELECT d.name AS n, t.plate AS p) SELECT 1 FROM u WHERE u.n <> u.p)
) AS trucks
FROM depots d;

-- name: GroupByOuterColumn :many
SELECT d.id, (SELECT count(*) FROM trucks t WHERE t.depot_id = d.id GROUP BY d.id) AS trucks
FROM depots d;

-- name: SiblingCTEInNestedWith :many
SELECT d.id, (
  WITH firsts AS (SELECT d.name AS label),
       seconds AS (SELECT (SELECT firsts.label) AS label FROM firsts)
  SELECT seconds.label FROM seconds
) AS label
FROM depots d;

-- name: SiblingCTEShadowsTable :many
SELECT d.id, (
  WITH trucks AS (SELECT t.depot_id, t.plate AS label FROM trucks t),
       labelled AS (SELECT (SELECT trucks.label) AS label FROM trucks WHERE trucks.depot_id = d.id)
  SELECT labelled.label FROM labelled LIMIT 1
) AS label
FROM depots d;

-- name: OuterColumnOfUpdateFrom :many
UPDATE depots SET note = 'seen'
FROM trucks t LEFT JOIN trackers k ON k.truck_id = t.id
WHERE t.depot_id = depots.id
RETURNING depots.id, (SELECT k.active) AS active;

-- name: OuterColumnOfSelfJoin :many
SELECT (SELECT k2.active) AS other_active, (SELECT trackers.active) AS active
FROM trackers
LEFT JOIN trackers AS k2 ON k2.truck_id = trackers.truck_id AND k2.id <> trackers.id;
