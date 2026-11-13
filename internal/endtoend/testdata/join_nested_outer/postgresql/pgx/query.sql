-- name: LeftThenRight :many
SELECT t.plate, k.active, d.name
FROM trucks t
LEFT JOIN trackers k ON k.truck_id = t.id
RIGHT JOIN depots d ON d.id = t.depot_id;

-- name: InnerThenFull :many
SELECT t.plate, k.active, d.name
FROM trucks t
JOIN trackers k ON k.truck_id = t.id
FULL JOIN depots d ON d.id = t.depot_id;

-- name: ParenthesisedJoin :many
SELECT d.name, t.plate, k.active
FROM depots d
LEFT JOIN (trucks t JOIN trackers k ON k.truck_id = t.id) ON t.depot_id = d.id;

-- name: LeftThenLeft :many
SELECT d.name, t.plate, k.active
FROM depots d
LEFT JOIN trucks t ON t.depot_id = d.id
LEFT JOIN trackers k ON k.truck_id = t.id;
