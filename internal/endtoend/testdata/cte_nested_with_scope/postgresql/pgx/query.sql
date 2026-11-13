-- name: SameNameTwoDepths :many
WITH d AS (SELECT id, name FROM depots)
SELECT x.plate, d.name
FROM (WITH d AS (SELECT plate, depot_id FROM trucks) SELECT d.plate, d.depot_id FROM d) x
JOIN d ON d.id = x.depot_id;

-- name: SiblingScopes :many
SELECT a.name, b.plate
FROM (WITH d AS (SELECT id, name FROM depots) SELECT d.id, d.name FROM d) a
JOIN (WITH d AS (SELECT depot_id, plate FROM trucks) SELECT d.depot_id, d.plate FROM d) b ON b.depot_id = a.id;

-- name: InnerReadsOuter :many
WITH o AS (SELECT id, note FROM depots)
SELECT t.plate, x.note
FROM trucks t
JOIN (WITH i AS (SELECT o.id, o.note FROM o) SELECT i.id, i.note FROM i) x ON x.id = t.depot_id;

-- name: ShadowsTable :many
SELECT x.id, x.label
FROM (WITH depots AS (SELECT id, plate AS label FROM trucks) SELECT * FROM depots) x;
