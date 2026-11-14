-- name: SubqueryInFromReadsSibling :many
SELECT x.n
FROM depots d, (WITH u AS (SELECT d.id AS n) SELECT u.n FROM u) x;

-- name: FunctionColumnIsNotOuter :many
SELECT s.id, (SELECT value FROM jsonb_each_text(s.value) LIMIT 1) AS first
FROM settings s;

-- name: LateralSiblingIsNotOuter :many
SELECT d.id, (SELECT x.truck FROM trucks t, LATERAL (SELECT id AS truck) x LIMIT 1) AS truck
FROM depots d;

-- name: AmbiguousIsNotOuter :many
SELECT k.id, (SELECT (SELECT id) FROM depots d JOIN trucks t ON t.depot_id = d.id LIMIT 1) AS other
FROM trackers k;

-- name: ShadowingAliasIsNotOuter :many
SELECT d.id, (SELECT d.name FROM trucks d LIMIT 1) AS name
FROM depots d;

-- name: ColumnAliasesAreNotOuter :many
SELECT d.id, (SELECT name FROM trucks AS t(id, depot_id, name) LIMIT 1) AS plate
FROM depots d;

-- name: AmbiguousGroupByIsNotOuter :many
SELECT d.id, (SELECT count(*) FROM trucks t JOIN trackers k ON k.truck_id = t.id GROUP BY id) AS n
FROM depots d;

-- name: UnqualifiedIsNotResolved :many
SELECT d.id, (WITH u AS (SELECT name AS label) SELECT u.label FROM u) AS label
FROM depots d;

-- name: RenamedColumnsAreNotResolved :many
SELECT d.id, (SELECT (SELECT t.plate) FROM trucks AS t(plate, depot_id, id) LIMIT 1) AS plate
FROM depots d;

-- name: AliasedJoinHidesItsTables :many
SELECT d.id, (SELECT (SELECT d.name) FROM (trucks d JOIN trackers k ON k.truck_id = d.id) AS j LIMIT 1) AS name
FROM depots d;

-- name: UsingAliasIsNotOuter :many
SELECT j.id, (SELECT j.id FROM trucks t JOIN trackers k USING (id) AS j LIMIT 1) AS other
FROM depots j;

-- name: ThreePartNameIsNotResolved :many
SELECT depots.id, (SELECT (SELECT public.depots.name) FROM trucks AS depots LIMIT 1) AS name
FROM public.depots;
