-- name: SubqueryInJoinCondition :many
SELECT *
FROM trucks t
JOIN depots d ON d.id = t.depot_id AND t.id IN (SELECT k.truck_id FROM trackers k);

-- name: NestedWithInJoinCondition :many
SELECT t.id, d.name
FROM trucks t
LEFT JOIN depots d ON d.id = t.depot_id
  AND t.id IN (WITH live AS (SELECT truck_id FROM trackers WHERE active) SELECT live.truck_id FROM live);
