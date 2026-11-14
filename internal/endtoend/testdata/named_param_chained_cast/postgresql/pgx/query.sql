-- name: ChainedText :many
SELECT id FROM trucks WHERE plate = @plate::text::text;

-- name: ChainedInteger :many
SELECT id FROM trucks WHERE depot_id = @depot_id::int::bigint;

-- name: ChainedTwice :many
SELECT id FROM trucks WHERE plate = @plate::text::varchar::text AND depot_id = @depot_id::int::bigint;

-- name: SingleCast :many
SELECT id FROM trucks WHERE plate = @plate::text;
