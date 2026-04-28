package index

import (
	"fmt"
	"inverse_index/internal/roaring"
)

type Expr interface{ exprNode() }

type TermExpr struct{ Word string }
type AndExpr struct{ Left, Right Expr }
type OrExpr struct{ Left, Right Expr }
type NotExpr struct{ Operand Expr }

func (TermExpr) exprNode() {}
func (AndExpr) exprNode()  {}
func (OrExpr) exprNode()   {}
func (NotExpr) exprNode()  {}

func Term(word string) Expr { return TermExpr{Word: word} }
func And(l, r Expr) Expr    { return AndExpr{Left: l, Right: r} }
func Or(l, r Expr) Expr     { return OrExpr{Left: l, Right: r} }
func Not(operand Expr) Expr { return NotExpr{Operand: operand} }

func evalExpr(idx *InvertedIndex, expr Expr) (*roaring.Bitmap, error) {
	if expr == nil {
		return roaring.New(), nil
	}
	switch e := expr.(type) {
	case TermExpr:
		return idx.Lookup(e.Word), nil

	case AndExpr:
		l, err := evalExpr(idx, e.Left)
		if err != nil {
			return nil, err
		}
		r, err := evalExpr(idx, e.Right)
		if err != nil {
			return nil, err
		}
		return roaring.And(l, r), nil

	case OrExpr:
		l, err := evalExpr(idx, e.Left)
		if err != nil {
			return nil, err
		}
		r, err := evalExpr(idx, e.Right)
		if err != nil {
			return nil, err
		}
		return roaring.Or(l, r), nil

	case NotExpr:
		operand, err := evalExpr(idx, e.Operand)
		if err != nil {
			return nil, err
		}
		// NOT = universe \ operand
		return roaring.AndNot(idx.AllDocs(), operand), nil

	default:
		return nil, fmt.Errorf("unknown Expr type: %T", expr)
	}
}
