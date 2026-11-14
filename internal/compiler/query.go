package compiler

import (
	"github.com/sqlc-dev/sqlc/internal/metadata"
	"github.com/sqlc-dev/sqlc/internal/sql/ast"
	"github.com/sqlc-dev/sqlc/internal/sql/catalog"
)

type Function struct {
	Rel        *ast.FuncName
	ReturnType *ast.TypeName
	Outs       []*catalog.Argument
}

type Table struct {
	Rel     *ast.TableName
	Columns []*Column

	// from is the FROM item this table stands for when that item is a
	// subquery or a function call. It is nil for a named relation.
	from ast.Node
}

type Column struct {
	Name         string
	OriginalName string
	DataType     string
	NotNull      bool
	Unsigned     bool
	IsArray      bool
	ArrayDims    int
	Comment      string
	Length       *int
	IsNamedParam bool
	IsFuncCall   bool

	// XXX: Figure out what PostgreSQL calls `foo.id`
	Scope      string
	Table      *ast.TableName
	TableAlias string
	Type       *ast.TypeName
	EmbedTable *ast.TableName

	IsSqlcSlice bool // is this sqlc.slice()

	skipTableRequiredCheck bool

	// from is the subquery or function call in FROM that this column was read
	// from, if any. A column read from one has no table name to look for in
	// the join tree, so outer-join nullability matches it by this instead.
	from ast.Node

	// byName is set on a column of a statement further out. Such a column is
	// always read as t.x, and outer-join nullability matches it to the table
	// that goes by the name t there.
	byName bool
}

type Query struct {
	SQL      string
	Metadata metadata.Metadata
	Columns  []*Column
	Params   []Parameter

	// Needed for CopyFrom
	InsertIntoTable *ast.TableName

	// Needed for vet
	RawStmt *ast.RawStmt
}

type Parameter struct {
	Number int
	Column *Column
}
