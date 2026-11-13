-- name: LeftJoinSubquery :many
SELECT t.id, s.active, s.serial
FROM trucks t
LEFT JOIN (SELECT truck_id, active, serial FROM trackers) s ON s.truck_id = t.id;

-- name: LeftJoinGrouped :many
SELECT d.name, s.trucks
FROM depots d
LEFT JOIN (SELECT depot_id, count(*) AS trucks FROM trucks GROUP BY depot_id) s ON s.depot_id = d.id;

-- name: LeftJoinLateral :many
SELECT d.name, l.plate
FROM depots d
LEFT JOIN LATERAL (
  SELECT t.plate FROM trucks t WHERE t.depot_id = d.id ORDER BY t.plate LIMIT 1
) l ON true;

-- name: RightJoinSubquery :many
SELECT s.active, t.plate
FROM (SELECT truck_id, active FROM trackers) s
RIGHT JOIN trucks t ON s.truck_id = t.id;

-- name: FullJoinSubquery :many
SELECT s.active, t.plate
FROM trucks t
FULL JOIN (SELECT truck_id, active FROM trackers) s ON s.truck_id = t.id;

-- name: InnerJoinSubquery :many
SELECT t.id, s.active
FROM trucks t
JOIN (SELECT truck_id, active FROM trackers) s ON s.truck_id = t.id;

-- name: SubqueryOnPreservedSide :many
SELECT s.active, t.plate
FROM (SELECT truck_id, active FROM trackers) s
LEFT JOIN trucks t ON s.truck_id = t.id;

-- name: LeftJoinSubqueryStar :many
SELECT *
FROM trucks t
LEFT JOIN (SELECT truck_id, active FROM trackers) s ON s.truck_id = t.id;

-- name: LeftJoinSubqueryCoalesce :many
SELECT t.id, coalesce(s.serial, 'none') AS serial
FROM trucks t
LEFT JOIN (SELECT truck_id, serial FROM trackers) s ON s.truck_id = t.id;

-- name: SubqueryOverJoinedTable :many
SELECT x.plate, t.id
FROM (SELECT id, plate FROM trucks) x
LEFT JOIN trucks t ON t.id = x.id + 1;

-- name: ScalarSubqueryOverSubquery :many
SELECT t.id, (SELECT s.id FROM (SELECT id FROM trackers) s LIMIT 1) AS tracker_id
FROM trucks t
LEFT JOIN trackers ON trackers.truck_id = t.id;
