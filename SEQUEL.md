# The `sequel` branch

This branch is sqlc v1.31.1 plus fixes to its query analysis. It exists so
that [sequel](https://github.com/Bockit/sequel) can pin a sqlc binary with these fixes. It is not
an official sqlc release, and sqlc's maintainers are not responsible for it.

Everything else is sqlc as released. For sqlc itself, see [README.md](README.md) and
<https://github.com/sqlc-dev/sqlc>.

## What it changes

Each fix has its own commits, with its tests under `internal/endtoend/testdata`.

1. **A `WITH` nested in a statement is analysed.** A `WITH` inside a `FROM` or `LATERAL`
   subquery, another CTE, or a subquery of the select list, `WHERE` or a join condition used to
   fail with `relation "x" does not exist`
   ([sqlc#3156](https://github.com/sqlc-dev/sqlc/issues/3156)). With the database analyzer on,
   that failure was silent: columns from the nullable side of an outer join came back as never
   null, and with a `*` in the statement the named parameters reached the database unrewritten.
2. **A column from an outer-joined subquery or function is nullable.** `LEFT JOIN (SELECT …) s`
   and `LEFT JOIN LATERAL (…) l` reported the columns of `s` and `l` as never null.
3. **An outer join makes the joins nested under it nullable.** `a LEFT JOIN b … RIGHT JOIN c`
   reported `a` as never null, and so did `a LEFT JOIN (b JOIN c ON …) ON …` for `b` and `c`.
4. **A subquery in a join condition is not part of the `FROM` clause.** Its tables were read as
   tables of the outer query, so a `*` expanded to their columns too.
5. **A CTE's body sees only the CTEs before it.** When a later CTE of the same `WITH` list, or
   the CTE itself, had the name of a table the body reads, a `*` in the body was expanded from
   the CTE and not from the table.
6. **A scalar subquery that can find no row is nullable.** `(SELECT name FROM depots WHERE …)`
   took the column's `NOT NULL` and was reported as never null. It stays never null only when the
   subquery always returns a row: an aggregate with no `GROUP BY`, or a `SELECT` with no `FROM`.
   The first of its two commits is upstream pull request 4654 by Ken Britton, cherry-picked; the
   second keeps an expression over `count`, as in `count(*)::int`, never null.
7. **A chained cast after a named parameter is rewritten.** `@name::text::text` came out as
   `$1xt`, and `@name::int::bigint` was not rewritten at all.
8. **`NULLIF` can be NULL.** It was reported as never null.
9. **A column read through a subquery or CTE is still a function call.** The `is_func_call` flag
   was dropped on the way out, so a plugin trusted the `NOT NULL` that sqlc gives a function
   call without knowing: `max(x)` over no rows is NULL.
10. **A nested statement can read the query around it.** A subquery in an expression whose
    select list, or whose `WITH`, names a column of the statement it is nested in used to fail
    with `column "x" does not exist`. A qualified reference, `t.x`, now goes to the nearest
    statement around it with a `FROM` item named `t`, as PostgreSQL decides it. An unqualified
    name still fails, and so does anything sqlc can't place with certainty: a reference from
    inside a `LATERAL` subquery, or to a function, to `VALUES`, or to an item with a list of
    column aliases. PostgreSQL only.

A query none of these apply to produces the output v1.31.1 produces. One query that used to
generate can now fail: a nested CTE that has the name of a table, and a body sqlc can't analyse.
v1.31.1 never looked at such a CTE and read the table in its place.

## Building it

sqlc v1.31.1 was released from Go 1.26.2 with cgo off. The same build, with `-trimpath`, is
reproducible: the same commit gives the same bytes on any machine, for any target.

```sh
CGO_ENABLED=0 GOFLAGS=-buildvcs=false GOOS=linux GOARCH=amd64 \
  go build -trimpath \
  -ldflags "-s -w -X github.com/sqlc-dev/sqlc/internal/cmd.version=v1.31.1-sequel.2" \
  -o sqlc ./cmd/sqlc
```

`GOOS` and `GOARCH` are `darwin`/`arm64`, `linux`/`amd64` or `linux`/`arm64`. Use Go 1.26.2
exactly (`GOTOOLCHAIN=local`), or the bytes will differ.

## Licence

MIT, as sqlc is. See [LICENSE](LICENSE).
