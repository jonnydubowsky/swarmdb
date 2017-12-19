package swarmdb

import (
	"fmt"
	//"github.com/ethereum/go-ethereum/log"
	"github.com/xwb1989/sqlparser"

)

//at the moment, only parses a query with a single un-nested where clause, i.e.
//'Select name, age from contacts where email = "rodney@wolk.com"'
func ParseQuery(rawquery string) (query Query, err error) {

	stmt, err := sqlparser.Parse(rawquery)
	if err != nil {
		fmt.Printf("sqlparser.Parse err: %v\n", err)
		return query, err
	}

	switch stmt := stmt.(type) {
	case *sqlparser.Select:
		//buf := sqlparser.NewTrackedBuffer(nil)
		//stmt.Format(buf)
		//fmt.Printf("select: %v\n", buf.String())

		query.Type = "Select"
		for i, column := range stmt.SelectExprs {
			fmt.Printf("select %d: %+v\n", i, sqlparser.String(column)) // stmt.(*sqlparser.Select).SelectExprs)
			var newcolumn Column
			newcolumn.ColumnName = sqlparser.String(column)
			//should somehow get IndexType, ColumnType, Primary from table itself...(not here?)
			query.RequestColumns = append(query.RequestColumns, newcolumn)
		}

		//From
		fmt.Printf("from 0: %+v \n", sqlparser.String(stmt.From[0]))
		query.Table = sqlparser.String(stmt.From[0])
		
		//Where & Having
		fmt.Printf("where or having: %s \n", readable(stmt.Where.Expr))
		if stmt.Where.Type == sqlparser.WhereStr { //Where

			fmt.Printf("type: %s\n", stmt.Where.Type)
			query.Where, err = parseWhere(stmt.Where.Expr)
			//this is where recursion for nested parentheses should take place
			if err != nil {
				return query, err
			}
			return query, err

		} else if stmt.Where.Type == sqlparser.HavingStr { //Having
			fmt.Printf("type: %s\n", stmt.Where.Type)
			//fill in having
		}


		//GroupBy ([]Expr)
		for _, g := range stmt.GroupBy {
			fmt.Printf("groupby: %s \n", readable(g))
		}

		//OrderBy

		//Limit

		/* Other options inside Select:
		   type Select struct {
		   	Cache       string
		   	Comments    Comments
		   	Distinct    string
		   	Hints       string
		   	SelectExprs SelectExprs
		   	From        TableExprs
		   	Where       *Where
		   	GroupBy     GroupBy
		   	Having      *Where
		   	OrderBy     OrderBy
		   	Limit       *Limit
		   	Lock        string
		   }*/

	case *sqlparser.Insert:
		query.Type = "Insert"
		//fill in

	case *sqlparser.Update:
		query.Type = "Update"
		//fill in

	case *sqlparser.Delete:
		query.Type = "Delete"
		//fill in

		/* Other Options for type of Query:
		   func (*Union) iStatement()      {}
		   func (*Select) iStatement()     {}
		   func (*Insert) iStatement()     {}
		   func (*Update) iStatement()     {}
		   func (*Delete) iStatement()     {}
		   func (*Set) iStatement()        {}
		   func (*DDL) iStatement()        {}
		   func (*Show) iStatement()       {}
		   func (*Use) iStatement()        {}
		   func (*OtherRead) iStatement()  {}
		   func (*OtherAdmin) iStatement() {}
		*/

	}

	return query, err
}

func parseWhere(expr sqlparser.Expr) (where Where, err error) {

	switch expr := expr.(type) {
	case *sqlparser.OrExpr:
		where.Left = readable(expr.Left)
		where.Right = readable(expr.Right)
		where.Operator = "OR" //should be const
	case *sqlparser.AndExpr:
		where.Left = readable(expr.Left)
		where.Right = readable(expr.Right)
		where.Operator = "AND" //shoud be const
	case *sqlparser.IsExpr:
		where.Right = readable(expr.Expr)
		where.Operator = expr.Operator
	case *sqlparser.BinaryExpr:
		where.Left = readable(expr.Left)
		where.Right = readable(expr.Right)
		where.Operator = expr.Operator
	case *sqlparser.ComparisonExpr:
		where.Left = readable(expr.Left)
		where.Right = readable(expr.Right)
		where.Operator = expr.Operator
	default:
		err = fmt.Errorf("WHERE expression not found")
	}
	return where, err
}

func readable(expr sqlparser.Expr) string {
	switch expr := expr.(type) {
	case *sqlparser.OrExpr:
		return fmt.Sprintf("(%s or %s)", readable(expr.Left), readable(expr.Right))
	case *sqlparser.AndExpr:
		return fmt.Sprintf("(%s and %s)", readable(expr.Left), readable(expr.Right))
	case *sqlparser.BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", readable(expr.Left), expr.Operator, readable(expr.Right))
	case *sqlparser.IsExpr:
		return fmt.Sprintf("(%s %s)", readable(expr.Expr), expr.Operator)
	case *sqlparser.ComparisonExpr:
		return fmt.Sprintf("(%s %s %s)", readable(expr.Left), expr.Operator, readable(expr.Right))
	default:
		return sqlparser.String(expr)
	}
}