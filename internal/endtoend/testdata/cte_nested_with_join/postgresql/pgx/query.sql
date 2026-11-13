-- name: NestedInJoin :many
SELECT t.id, k.active, k.serial
FROM trucks t
LEFT JOIN trackers k ON k.truck_id = t.id
JOIN (WITH d AS (SELECT id FROM depots) SELECT d.id FROM d) x ON x.id = t.depot_id;

-- name: NestedBodyLeftJoin :many
SELECT x.plate, x.active
FROM (
  WITH j AS (
    SELECT t.plate, k.active FROM trucks t LEFT JOIN trackers k ON k.truck_id = t.id
  )
  SELECT j.plate, j.active FROM j
) x;

-- name: NestedInCTE :many
WITH busy AS (
  WITH tracked AS (SELECT t.id, t.depot_id FROM trucks t JOIN trackers k ON k.truck_id = t.id)
  SELECT tracked.id, tracked.depot_id FROM tracked
)
SELECT d.name, b.id AS truck_id
FROM depots d
LEFT JOIN busy b ON b.depot_id = d.id;

-- name: NestedStarAndParam :many
SELECT t.*
FROM trucks t
JOIN (WITH d AS (SELECT * FROM depots WHERE name <> @skip) SELECT * FROM d) x ON x.id = t.depot_id
WHERE t.plate = @plate;

-- name: NestedPositional :many
SELECT t.id, t.plate
FROM trucks t
WHERE t.depot_id IN (
  WITH RECURSIVE d AS (
    SELECT id FROM depots WHERE depots.name = $1
    UNION ALL
    SELECT d.id + 1 FROM d WHERE d.id < 0
  )
  SELECT d.id FROM d
);

-- name: NestedScalar :many
SELECT t.id, (WITH d AS (SELECT id FROM depots) SELECT count(*) FROM d WHERE d.id = t.depot_id) AS depots
FROM trucks t;

-- name: NestedInsert :one
INSERT INTO trackers (truck_id, active, serial)
SELECT x.id, true, @serial
FROM (WITH t AS (SELECT id FROM trucks WHERE plate = @plate) SELECT t.id FROM t) x
RETURNING *;
