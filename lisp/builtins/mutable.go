package builtins

import (
	"fmt"

	"go.spiff.io/skim/lisp/interp"
	"go.spiff.io/skim/lisp/skim"
)

func SetQuoted(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return nil, nil
	}
	var (
		a    skim.Atom = form
		name skim.Atom
		sym  skim.Symbol
		ok   bool
	)
	for ; err == nil && a != nil; a, err = skim.Cddr(a) {
		name, err = skim.Car(a)
		if err != nil {
			return nil, err
		}
		sym, ok = name.(skim.Symbol)
		if !ok {
			return nil, fmt.Errorf("setq: cannot assign to non-symbol type %T", name)
		}

		if result, err = skim.Cadr(a); err != nil {
			return nil, err
		} else if result, err = ctx.Eval(result); err != nil {
			return nil, err
		}
		ctx.Bind(sym, result)
	}
	if err != nil {
		result = nil
	}
	return result, err
}

func SetUnquoted(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return nil, nil
	}
	var (
		a    skim.Atom = form
		name skim.Atom
		sym  skim.Symbol
		ok   bool
	)
	for ; err == nil && a != nil; a, err = skim.Cddr(a) {
		name, err = skim.Car(a)
		if err != nil {
			return nil, err
		} else if name, err = ctx.Eval(name); err != nil {
			return nil, err
		}

		sym, ok = name.(skim.Symbol)
		if !ok {
			return nil, fmt.Errorf("set: cannot assign to non-symbol type %T", name)
		}

		if result, err = skim.Cadr(a); err != nil {
			return nil, err
		} else if result, err = ctx.Eval(result); err != nil {
			return nil, err
		}
		ctx.Bind(sym, result)
	}
	if err != nil {
		result = nil
	}
	return result, err
}

func UnbindQuoted(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	err = skim.Walk(form, func(a skim.Atom) error {
		sym, ok := a.(skim.Symbol)
		if !ok {
			return fmt.Errorf("unbindq: cannot unbind non-symbol type %T", a)
		}
		result = sym
		ctx.Unbind(sym)
		return nil
	})
	if err != nil {
		result = nil
	}
	return result, err
}

func UnbindUnquoted(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	err = skim.Walk(form, func(a skim.Atom) (err error) {
		if a, err = ctx.Eval(a); err != nil {
			return err
		}
		sym, ok := a.(skim.Symbol)
		if !ok {
			return fmt.Errorf("unbindq: cannot unbind non-symbol type %T", a)
		}
		result = sym
		ctx.Unbind(sym)
		return nil
	})
	if err != nil {
		result = nil
	}
	return result, err
}

func BindMutative(ctx *interp.Context) {
	// TODO: setf, if records are ever supported
	ctx.BindProc("set", interp.Proc(SetUnquoted))
	ctx.BindProc("setq", interp.Proc(SetQuoted))
	ctx.BindProc("unbindq", interp.Proc(UnbindQuoted))
	ctx.BindProc("unbind", interp.Proc(UnbindUnquoted))
}
