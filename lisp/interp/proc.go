package interp

import (
	"fmt"

	"go.spiff.io/skim/lisp/skim"
)

type Proc func(*Context, *skim.Cons) (skim.Atom, error)

var _ Evaler = Proc(nil)

func (Proc) SkimAtom() {}
func (p Proc) String() string {
	if p == nil {
		return "proc#nil"
	}
	return fmt.Sprintf("proc#%p", p)
}

func (p Proc) Eval(ctx *Context, form *skim.Cons) (skim.Atom, error) {
	if p == nil {
		return nil, fmt.Errorf("skim: proc is nil")
	}
	return p(ctx, form)
}
