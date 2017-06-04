package builtins

import (
	"fmt"
	"io"
	"os"

	"go.spiff.io/skim/lisp/interp"
	"go.spiff.io/skim/lisp/skim"
)

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

	return result, nil
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
	ctx.BindProc("quote", QuoteFn)
}

func BindDisplay(ctx *interp.Context) {
	ctx.BindProc("newline", Newline)
	ctx.BindProc("display", Display)
}
