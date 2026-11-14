package compiler

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/sqlc-dev/sqlc/internal/config"
	"github.com/sqlc-dev/sqlc/internal/sql/ast"
	"github.com/sqlc-dev/sqlc/internal/sql/astutils"
	"github.com/sqlc-dev/sqlc/internal/sql/catalog"
	"github.com/sqlc-dev/sqlc/internal/sql/rewrite"
	"github.com/sqlc-dev/sqlc/internal/sql/sqlerr"
)

type QueryCatalog struct {
	catalog *catalog.Catalog
	ctes    map[string]*Table
	embeds  rewrite.EmbedSet

	// A statement nested in another one can have a WITH clause of its own.
	// Its names are visible inside it and nowhere else, and they shadow the
	// names of the statements around it. Each such statement gets a scope: a
	// QueryCatalog holding its own CTEs, whose parent is the catalog of the
	// statement that contains it.
	//
	// parent is nil for the catalog of the outermost statement.
	parent *QueryCatalog
	// stmt is the statement whose WITH clause filled ctes.
	stmt ast.Node
	// scopes holds the scope of every nested statement seen so far, keyed by
	// the statement. Every catalog built for one query shares it.
	scopes map[ast.Node]*QueryCatalog
	// bodies holds, for every CTE seen so far, the catalog its body is read
	// with: the catalog of the statement its WITH clause belongs to, limited
	// to the CTEs that come before it, and itself when the clause is
	// RECURSIVE. Every catalog built for one query shares it.
	bodies map[*ast.CommonTableExpr]*QueryCatalog
	// parents holds, for every statement nested in the query, the statement
	// it sits in and how. Every catalog built for one query shares it.
	parents map[ast.Node]stmtParent
	// tables remembers the FROM clause of each statement an outer reference
	// was resolved against. Every catalog built for one query shares it.
	tables map[tablesKey][]*Table
}

// How a nested statement sits in the statement around it.
const (
	// A branch of a set operation, or the SELECT of an INSERT: the same query
	// level as its parent.
	nestedDirect = iota
	// A subquery in an expression: it can read the columns of its parent's
	// FROM clause.
	nestedSubLink
	// A subquery in an expression that is itself part of the FROM clause, as
	// a join condition or a function's argument is. It can read some of its
	// parent's FROM items; which ones isn't modelled here, so a reference to
	// any of them is never resolved.
	nestedFromExpr
	// A subquery in FROM, or the body of a CTE: it can't read its parent's FROM
	// clause, only what the parent itself can read from further out.
	nestedFrom
	// A LATERAL subquery in FROM: it reads the FROM items before it ahead of
	// anything further out. Which items those are isn't modelled here, so an
	// outer reference from inside one is never resolved.
	nestedLateral
)

type tablesKey struct {
	stmt ast.Node
	qc   *QueryCatalog
	ctes int
}

type stmtParent struct {
	stmt ast.Node
	how  int
}

// parentWalker records the parent of every nested statement.
type parentWalker struct {
	parents map[ast.Node]stmtParent
	stmt    ast.Node
	how     int
	// from holds the FROM clauses of stmt, and inFrom is set under them.
	from   []*ast.List
	inFrom bool
}

func (w *parentWalker) Visit(node ast.Node) astutils.Visitor {
	switch n := node.(type) {
	case *ast.SelectStmt, *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt:
		if w.stmt != nil {
			w.parents[node] = stmtParent{stmt: w.stmt, how: w.how}
		}
		next := &parentWalker{parents: w.parents, stmt: node, how: nestedDirect}
		switch stmt := node.(type) {
		case *ast.SelectStmt:
			next.from = []*ast.List{stmt.FromClause}
		case *ast.UpdateStmt:
			next.from = []*ast.List{stmt.FromClause}
		case *ast.DeleteStmt:
			next.from = []*ast.List{stmt.UsingClause}
		}
		return next
	case *ast.List:
		if n != nil && !w.inFrom {
			for _, from := range w.from {
				if from == n {
					next := *w
					next.inFrom = true
					return &next
				}
			}
		}
	case *ast.SubLink:
		how := nestedSubLink
		if w.inFrom {
			how = nestedFromExpr
		}
		return &parentWalker{parents: w.parents, stmt: w.stmt, how: how}
	case *ast.RangeSubselect:
		if n.Lateral {
			return &parentWalker{parents: w.parents, stmt: w.stmt, how: nestedLateral}
		}
		return &parentWalker{parents: w.parents, stmt: w.stmt, how: nestedFrom}
	case *ast.CommonTableExpr:
		return &parentWalker{parents: w.parents, stmt: w.stmt, how: nestedFrom}
	}
	return w
}

// catalogOf returns the catalog in effect for a statement around the one qc is
// the catalog of.
func (qc *QueryCatalog) catalogOf(stmt ast.Node) *QueryCatalog {
	root := qc
	for root.parent != nil {
		root = root.parent
	}
	for cur := stmt; cur != nil; {
		// The catalogs qc is nested in come first: while a WITH clause is being
		// read, and inside a CTE's body afterwards, they hold only the CTEs
		// that are visible from here.
		for s := qc; s != nil; s = s.parent {
			if s.stmt == cur {
				return s
			}
		}
		if scope, ok := qc.scopes[cur]; ok {
			return scope
		}
		p, ok := qc.parents[cur]
		if !ok {
			break
		}
		cur = p.stmt
	}
	return root
}

// fromShape holds the names the items of a statement's FROM clause go by.
// PostgreSQL gives a qualified reference, t.x, to the nearest statement that
// has an item named t, whether or not the item has the column, so the names
// alone decide which statement a reference belongs to.
type fromShape struct {
	// names has the name of each FROM item: its alias, or else the name of
	// its table or function. The value says whether sqlc knows the item's
	// columns as PostgreSQL does. It doesn't for a function, VALUES, an item
	// with a list of column aliases, an item under an aliased join, which
	// hides it, or a name that two items share.
	names map[string]bool
	// unknown is set when the clause has an item whose name can't be read.
	unknown bool
}

func (s *fromShape) name(name *string, known bool) {
	if name == nil {
		s.unknown = true
		return
	}
	if _, dup := s.names[*name]; dup {
		known = false
	}
	s.names[*name] = known
}

func (s *fromShape) add(node ast.Node, hidden bool) {
	renamed := func(alias *ast.Alias) bool {
		return alias != nil && alias.Colnames != nil && len(alias.Colnames.Items) > 0
	}
	switch n := node.(type) {
	case nil:
	case *ast.List:
		if n == nil {
			return
		}
		for _, item := range n.Items {
			s.add(item, hidden)
		}
	case *ast.RangeVar:
		if n == nil {
			return
		}
		name := n.Relname
		if n.Alias != nil {
			name = n.Alias.Aliasname
		}
		s.name(name, !hidden && !renamed(n.Alias))
	case *ast.RangeSubselect:
		if n.Alias == nil {
			// A subquery with no alias has no name to be read by.
			return
		}
		sel, ok := n.Subquery.(*ast.SelectStmt)
		values := ok && sel.ValuesLists != nil && len(sel.ValuesLists.Items) > 0
		s.name(n.Alias.Aliasname, !hidden && ok && !values && !renamed(n.Alias))
	case *ast.RangeFunction:
		if n.Alias != nil {
			s.name(n.Alias.Aliasname, false)
			return
		}
		var call ast.Node
		if n.Functions != nil && len(n.Functions.Items) > 0 {
			call = n.Functions.Items[0]
			if list, ok := call.(*ast.List); ok && list != nil && len(list.Items) > 0 {
				call = list.Items[0]
			}
		}
		if fn, ok := call.(*ast.FuncCall); ok && fn.Func != nil {
			s.name(&fn.Func.Name, false)
		} else {
			s.unknown = true
		}
	case *ast.JoinExpr:
		if n.Alias != nil {
			s.name(n.Alias.Aliasname, false)
			hidden = true
		}
		if n.UsingAlias != nil {
			s.name(n.UsingAlias.Aliasname, false)
		}
		s.add(n.Larg, hidden)
		s.add(n.Rarg, hidden)
	default:
		s.unknown = true
	}
}

// shapeOf reads the names in the FROM clause of stmt.
func shapeOf(stmt ast.Node) fromShape {
	s := fromShape{names: map[string]bool{}}
	switch n := stmt.(type) {
	case *ast.SelectStmt:
		s.add(n.FromClause, false)
	case *ast.UpdateStmt:
		s.add(n.Relations, false)
		s.add(n.FromClause, false)
	case *ast.DeleteStmt:
		s.add(n.Relations, false)
		// sourceTables doesn't read USING.
		s.add(n.UsingClause, true)
		if n.FromClause != nil {
			s.unknown = true
		}
	case *ast.InsertStmt:
		if n.Relation != nil {
			s.add(n.Relation, false)
		}
		if n.OnConflictClause != nil {
			excluded := "excluded"
			s.name(&excluded, false)
		}
	default:
		s.unknown = true
	}
	return s
}

// outerTables returns the FROM clause of stmt, a statement around the one qc
// is the catalog of.
func (comp *Compiler) outerTables(qc *QueryCatalog, stmt ast.Node) ([]*Table, error) {
	cat := qc.catalogOf(stmt)
	key := tablesKey{stmt: stmt, qc: cat, ctes: len(cat.ctes)}
	if tables, ok := qc.tables[key]; ok {
		return tables, nil
	}
	tables, err := comp.sourceTables(cat, stmt)
	if err != nil {
		return nil, err
	}
	if qc.tables != nil {
		qc.tables[key] = tables
	}
	return tables, nil
}

// outerColumnRefs resolves a column reference that node's own FROM clause
// doesn't have, as a reference to the query around it: a subquery in an
// expression can read the columns of the statements it is nested in.
//
// Only a qualified reference, t.x, is resolved, and only in PostgreSQL. It
// belongs to the nearest statement with a FROM item named t that node can
// see. An unqualified name belongs to the nearest statement with a column
// of that name, and sqlc's list of a FROM clause's columns isn't complete
// enough to tell which statement that is.
//
// It reports false if no statement around node has an item of that name,
// and also wherever sqlc can't tell what PostgreSQL would do. The caller's
// error then stands, as it did before outer references were resolved at all.
func (comp *Compiler) outerColumnRefs(qc *QueryCatalog, node ast.Node, res *ast.ResTarget, ref *ast.ColumnRef, cause error) ([]*Column, bool) {
	if qc == nil || qc.parents == nil || !isMissingColumn(cause) {
		return nil, false
	}
	if comp.conf.Engine != config.EnginePostgreSQL {
		return nil, false
	}
	parts := stringSlice(ref.Fields)
	if len(parts) != 2 {
		return nil, false
	}
	table := parts[0]
	// node's own FROM clause answers for the name if it has it.
	if own := shapeOf(node); own.unknown {
		return nil, false
	} else if _, ok := own.names[table]; ok {
		return nil, false
	}
	cur := node
	for range len(qc.parents) + 1 {
		p, ok := qc.parents[cur]
		if !ok {
			return nil, false
		}
		cur = p.stmt
		switch p.how {
		case nestedLateral:
			return nil, false
		case nestedDirect, nestedFrom:
			continue
		}
		shape := shapeOf(p.stmt)
		if shape.unknown {
			return nil, false
		}
		known, ok := shape.names[table]
		if !ok {
			continue
		}
		if !known || p.how != nestedSubLink {
			return nil, false
		}
		tables, err := comp.outerTables(qc, p.stmt)
		if err != nil {
			return nil, false
		}
		cols, err := outputColumnRefs(res, tables, ref)
		if err != nil {
			return nil, false
		}
		// The column is nullable as the statement it belongs to joins it,
		// and node's own joins have no say.
		var from *ast.List
		switch n := p.stmt.(type) {
		case *ast.SelectStmt:
			from = n.FromClause
		case *ast.UpdateStmt:
			from = n.FromClause
		}
		for _, col := range cols {
			col.byName = true
			if from != nil && col.NotNull && (col.Table != nil || col.from != nil) {
				for _, f := range from.Items {
					if res := isTableRequired(f, col, tableRequired); res != tableNotFound {
						col.NotNull = res == tableRequired
						break
					}
				}
			}
			col.skipTableRequiredCheck = true
		}
		return cols, true
	}
	return nil, false
}

// outerHasColumn reports whether item, which the lookup in node's own FROM
// clause and select list failed on with cause, is a column of a statement
// around node.
func (comp *Compiler) outerHasColumn(qc *QueryCatalog, node ast.Node, item ast.Node, cause error) bool {
	ref, ok := item.(*ast.ColumnRef)
	if !ok {
		return false
	}
	_, found := comp.outerColumnRefs(qc, node, &ast.ResTarget{}, ref, cause)
	return found
}

// isMissingColumn reports whether err says a column wasn't found, and not
// that it was found more than once.
func isMissingColumn(err error) bool {
	var sqlErr *sqlerr.Error
	if !errors.As(err, &sqlErr) || sqlErr.Code != "42703" {
		return false
	}
	return strings.HasSuffix(sqlErr.Message, "does not exist") || strings.HasSuffix(sqlErr.Message, "not found")
}

func withClause(node ast.Node) *ast.WithClause {
	switch n := node.(type) {
	case *ast.DeleteStmt:
		return n.WithClause
	case *ast.InsertStmt:
		return n.WithClause
	case *ast.UpdateStmt:
		return n.WithClause
	case *ast.SelectStmt:
		return n.WithClause
	}
	return nil
}

func (comp *Compiler) buildQueryCatalog(c *catalog.Catalog, node ast.Node, embeds rewrite.EmbedSet) (*QueryCatalog, error) {
	qc := &QueryCatalog{
		catalog: c,
		ctes:    map[string]*Table{},
		embeds:  embeds,
		stmt:    node,
		scopes:  map[ast.Node]*QueryCatalog{},
		bodies:  map[*ast.CommonTableExpr]*QueryCatalog{},
		parents: map[ast.Node]stmtParent{},
		tables:  map[tablesKey][]*Table{},
	}
	if node != nil {
		astutils.Walk(&parentWalker{parents: qc.parents}, node)
	}
	if err := comp.addCTEs(qc, withClause(node)); err != nil {
		return nil, err
	}
	return qc, nil
}

// addCTEs registers the CTEs of one WITH clause in qc, in order, so that each
// one can read those before it.
func (comp *Compiler) addCTEs(qc *QueryCatalog, with *ast.WithClause) error {
	if with == nil {
		return nil
	}
	for _, item := range with.Ctes.Items {
		if cte, ok := item.(*ast.CommonTableExpr); ok {
			// The body sees the CTEs registered so far and none of the later
			// ones, now and when the statement is walked again afterwards.
			body := &QueryCatalog{
				catalog: qc.catalog,
				ctes:    maps.Clone(qc.ctes),
				embeds:  qc.embeds,
				parent:  qc.parent,
				stmt:    qc.stmt,
				scopes:  qc.scopes,
				bodies:  qc.bodies,
				parents: qc.parents,
				tables:  qc.tables,
			}
			cols, err := comp.outputColumns(body, cte.Ctequery)
			if err != nil {
				return err
			}
			var names []string
			if cte.Aliascolnames != nil {
				for _, item := range cte.Aliascolnames.Items {
					if val, ok := item.(*ast.String); ok {
						names = append(names, val.Str)
					} else {
						names = append(names, "")
					}
				}
			}
			rel := &ast.TableName{Name: *cte.Ctename}
			for i := range cols {
				cols[i].Table = rel
				if len(names) > i {
					cols[i].Name = names[i]
				}
			}
			table := &Table{
				Rel:     rel,
				Columns: cols,
			}
			qc.ctes[*cte.Ctename] = table
			if with.Recursive {
				body.ctes[*cte.Ctename] = table
			}
			if qc.bodies != nil {
				qc.bodies[cte] = body
			}
		}
	}
	return nil
}

// scopeFor returns the catalog to use inside node, where qc is the catalog in
// effect around it. That is qc itself unless node is a nested statement with a
// WITH clause of its own, in which case it is that statement's scope, built on
// first use.
//
// A query with no nested WITH clause never gets a second catalog.
func (comp *Compiler) scopeFor(qc *QueryCatalog, node ast.Node) (*QueryCatalog, error) {
	if qc == nil {
		return nil, nil
	}
	with := withClause(node)
	if with == nil {
		return qc, nil
	}
	for s := qc; s != nil; s = s.parent {
		if s.stmt == node {
			// Already inside this statement.
			return qc, nil
		}
	}
	if scope, ok := qc.scopes[node]; ok {
		return scope, nil
	}
	scope := &QueryCatalog{
		catalog: qc.catalog,
		ctes:    map[string]*Table{},
		embeds:  qc.embeds,
		parent:  qc,
		stmt:    node,
		scopes:  qc.scopes,
		bodies:  qc.bodies,
		parents: qc.parents,
		tables:  qc.tables,
	}
	if err := comp.addCTEs(scope, with); err != nil {
		return nil, err
	}
	qc.scopes[node] = scope
	return scope, nil
}

// scopeWalker walks a statement and reports each node with the catalog in
// effect at that node.
type scopeWalker struct {
	comp  *Compiler
	qc    *QueryCatalog
	visit func(qc *QueryCatalog, node ast.Node) error
	err   *error
}

func (w *scopeWalker) Visit(node ast.Node) astutils.Visitor {
	if node == nil {
		return w
	}
	if *w.err != nil {
		return nil
	}
	qc, err := w.comp.scopeFor(w.qc, node)
	if err != nil {
		*w.err = err
		return nil
	}
	if cte, ok := node.(*ast.CommonTableExpr); ok && qc != nil {
		// Inside a CTE, only the CTEs before it are visible.
		if body, ok := qc.bodies[cte]; ok {
			qc = body
		}
	}
	if err := w.visit(qc, node); err != nil {
		*w.err = err
		return nil
	}
	if qc == w.qc {
		return w
	}
	return &scopeWalker{comp: w.comp, qc: qc, visit: w.visit, err: w.err}
}

// walkScopes calls visit for every node under root, in the order of
// astutils.Walk, with the catalog in effect at that node.
func (comp *Compiler) walkScopes(qc *QueryCatalog, root ast.Node, visit func(qc *QueryCatalog, node ast.Node) error) error {
	var err error
	astutils.Walk(&scopeWalker{comp: comp, qc: qc, visit: visit, err: &err}, root)
	return err
}

func ConvertColumn(rel *ast.TableName, c *catalog.Column) *Column {
	return &Column{
		Table:     rel,
		Name:      c.Name,
		DataType:  dataType(&c.Type),
		NotNull:   c.IsNotNull,
		Unsigned:  c.IsUnsigned,
		IsArray:   c.IsArray,
		ArrayDims: c.ArrayDims,
		Type:      &c.Type,
		Length:    c.Length,
	}
}

func (qc QueryCatalog) GetTable(rel *ast.TableName) (*Table, error) {
	cte, exists := qc.ctes[rel.Name]
	if exists {
		return &Table{Rel: rel, Columns: cte.Columns}, nil
	}
	if qc.parent != nil {
		return qc.parent.GetTable(rel)
	}
	src, err := qc.catalog.GetTable(rel)
	if err != nil {
		return nil, err
	}
	var cols []*Column
	for _, c := range src.Columns {
		cols = append(cols, ConvertColumn(rel, c))
	}
	return &Table{Rel: rel, Columns: cols}, nil
}

func (qc QueryCatalog) GetFunc(rel *ast.FuncName) (*Function, error) {
	funcs, err := qc.catalog.ListFuncsByName(rel)
	if err != nil {
		return nil, err
	}
	if len(funcs) == 0 {
		return nil, fmt.Errorf("function not found: %s", rel.Name)
	}
	return &Function{
		Rel:        rel,
		Outs:       funcs[0].OutArgs(),
		ReturnType: funcs[0].ReturnType,
	}, nil
}
