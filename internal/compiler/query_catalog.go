package compiler

import (
	"fmt"

	"github.com/sqlc-dev/sqlc/internal/sql/ast"
	"github.com/sqlc-dev/sqlc/internal/sql/astutils"
	"github.com/sqlc-dev/sqlc/internal/sql/catalog"
	"github.com/sqlc-dev/sqlc/internal/sql/rewrite"
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
			cols, err := comp.outputColumns(qc, cte.Ctequery)
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
			qc.ctes[*cte.Ctename] = &Table{
				Rel:     rel,
				Columns: cols,
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
