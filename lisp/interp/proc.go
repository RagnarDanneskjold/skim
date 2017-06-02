package interp

import (
	"fmt"

	"go.spiff.io/skim/lisp/skim"
)

type Proc func(*Context, *skim.Cons) (skim.Atom, error)

func (Proc) SkimAtom() {}
func (p Proc) String() string {
	if p == nil {
		return "proc#nil"
	}
	return fmt.Sprintf("proc#%p", p)
}
