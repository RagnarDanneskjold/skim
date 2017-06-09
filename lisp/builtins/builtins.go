package builtins

import (
	"errors"
	"fmt"
	"io"
	"os"

	"go.spiff.io/skim/lisp/interp"
	"go.spiff.io/skim/lisp/skim"
)

// MapFunc is a function used to map an atom to another atom. It may return an error, in which case
// the caller should assume the result atom is unusable unless documented otherwise for special uses
// of a specific MapFunc.
type MapFunc func(skim.Atom) (skim.Atom, error)

func (fn MapFunc) Map(atom skim.Atom) (skim.Atom, error) {
	if fn == nil {
		return nil, errors.New("map: MapFunc is nil")
	}
	return fn(atom)
}

// Map iterates over a list and maps its values using mapfn. It returns a new list with the mapped
// values. The input list must be, strictly, a list -- that is, all Cdrs of the input list must
// either be nil or another cons cell meeting the same criteria.
func Map(list *skim.Cons, mapfn MapFunc) (*skim.Cons, error) {
	if list == nil {
		return nil, nil
	}

	var root skim.Atom
	var cdr *skim.Atom = &root
	err := skim.Walk(list, func(a skim.Atom) (err error) {
		if a, err = mapfn.Map(a); err != nil {
			return err
		}

		next := &skim.Cons{Car: a}
		*cdr, cdr = next, &next.Cdr
		return nil
	})

	if err != nil {
		return nil, err
	}
	return root.(*skim.Cons), nil
}

// Expand expands the values by evaluating each value in the scope of the interpreter context, ctx.
// It returns a new list with the expanded values.
//
// This is a convenience function for Map(list, ctx.Eval).
func Expand(ctx *interp.Context, list *skim.Cons) (*skim.Cons, error) {
	return Map(list, ctx.Eval)
}

// Expanded returns a new Proc that will invoke fn with expanded values of its form when called.
// This is useful as a convenience when dealing with regular functions that do not receive anything
// other than normal arguments as a list. For special procs, such as let, let*, begin, cond, and, or
// (particulary for short-circuiting), and so on, more careful evaluation of its arguments is
// necessary.
func Expanded(fn interp.Proc) interp.Proc {
	return func(ctx *interp.Context, argv *skim.Cons) (result skim.Atom, err error) {
		if argv, err = Expand(ctx, argv); err != nil {
			return nil, err
		}
		return fn(ctx, argv)
	}
}

func BeginBlock(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	err = skim.Walk(form, func(a skim.Atom) error { result, err = ctx.Eval(a); return err })
	if err != nil {
		result = nil
	}
	return
}

func letform(eval, bind *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	err = skim.Walk(form.Car, func(a skim.Atom) error {
		l, r, err := skim.Pair(a)
		if err != nil {
			return err
		}
		sym, ok := l.(skim.Symbol)
		if !ok {
			return fmt.Errorf("expected symbol, got %T", l)
		}

		r, err = eval.Eval(r)
		if err != nil {
			return err
		}
		bind.Bind(sym, r)
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = skim.Walk(form.Cdr, func(a skim.Atom) error {
		result, err = bind.Eval(a)
		return err
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func LogAnd(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return nil, nil
	}
	for a := skim.Atom(form); a != nil && err == nil; a, err = skim.Cdr(a) {
		result, err = skim.Car(a)
		if err == nil {
			result, err = ctx.Eval(result)
		}
		if err != nil {
			return nil, err
		}

		if !skim.IsTrue(result) {
			return nil, nil
		}
	}
	if err != nil {
		result = nil
	}
	return
}

func LogOr(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return nil, nil
	}
	for a := skim.Atom(form); a != nil && err == nil; a, err = skim.Cdr(a) {
		result, err = skim.Car(a)
		if err == nil {
			result, err = ctx.Eval(result)
		}
		if err != nil {
			return nil, err
		}

		if skim.IsTrue(result) {
			return result, nil
		}
	}
	return nil, err
}

func Cond(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return
	}

	var a skim.Atom = form
	for ; a != nil; a, err = skim.Cdr(a) {
		var clause, test, conseq skim.Atom
		clause, err = skim.Car(a)
		if err != nil {
			return nil, err
		}

		test, err = skim.Car(clause)
		if err != nil {
			return nil, err
		}

		conseq, err = skim.Cdr(clause)
		if err != nil {
			return nil, err
		}

		test, err = ctx.Eval(test)
		if err != nil {
			return nil, err
		} else if !skim.IsTrue(test) {
			continue
		}

		err = skim.Walk(conseq, func(a skim.Atom) error { result, err = ctx.Eval(a); return err })
		if err != nil {
			result = nil
		}
		return
	}
	return nil, nil
}

func Let(ctx *interp.Context, form *skim.Cons) (skim.Atom, error) {
	return letform(ctx, ctx.Fork(), form)
}

func LetStar(ctx *interp.Context, form *skim.Cons) (skim.Atom, error) {
	ctx = ctx.Fork()
	return letform(ctx, ctx, form)
}

func Newline(c *interp.Context, v *skim.Cons) (skim.Atom, error) {
	if v != nil {
		return nil, fmt.Errorf("expected no arguments; got %v", v)
	}
	_, err := io.WriteString(os.Stdout, "\n")
	return nil, err
}

func Display(c *interp.Context, v *skim.Cons) (_ skim.Atom, err error) {
	var args []interface{}
	err = skim.Walk(v, func(a skim.Atom) error {
		a, err := c.Eval(a)
		if err != nil {
		} else if str, ok := a.(skim.String); ok {
			args = append(args, string(str))
		} else {
			args = append(args, a)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, nil
	}
	_, err = fmt.Print(args...)
	return nil, err
}

func Cons(ctx *interp.Context, form *skim.Cons) (cons skim.Atom, err error) {
	car, cdr, err := skim.Pair(form)
	if err != nil {
		return nil, err
	}

	car, err = ctx.Eval(car)
	if err == nil {
		cdr, err = ctx.Eval(cdr)
	}
	if err != nil {
		return nil, err
	}
	return &skim.Cons{Car: car, Cdr: cdr}, nil
}

func List(ctx *interp.Context, form *skim.Cons) (list skim.Atom, err error) {
	if form == nil {
		return &skim.Cons{}, nil
	}
	var pred *skim.Atom = &list
	for a := skim.Atom(form); a != nil && err == nil; a, err = skim.Cdr(a) {
		var car skim.Atom
		car, err = skim.Car(a)
		if err == nil {
			car, err = ctx.Eval(car)
		}
		if err != nil {
			return nil, err
		}

		next := &skim.Cons{Car: car}
		*pred, pred = next, &next.Cdr
	}
	return list, nil
}

func QuoteFn(c *interp.Context, v *skim.Cons) (skim.Atom, error) {
	return v.Car, nil
}

func QuasiquoteFn(c *interp.Context, v *skim.Cons) (skim.Atom, error) {
	return c.Fork().BindProc("unquote", UnquoteFn).Eval(v.Car)
}

func UnquoteFn(c *interp.Context, v *skim.Cons) (skim.Atom, error) {
	return c.Fork().Bind("unquote", nil).Eval(v.Car)
}

func BindCore(ctx *interp.Context) {
	ctx.BindProc("begin", BeginBlock)
	ctx.BindProc("let", Let)
	ctx.BindProc("let*", LetStar)
	ctx.BindProc("cons", Cons)
	ctx.BindProc("list", List)
	ctx.BindProc("quote", QuoteFn)
	ctx.BindProc("cond", Cond)
	ctx.BindProc("and", LogAnd)
	ctx.BindProc("or", LogOr)
}

func BindDisplay(ctx *interp.Context) {
	ctx.BindProc("newline", Newline)
	ctx.BindProc("display", Display)
}
