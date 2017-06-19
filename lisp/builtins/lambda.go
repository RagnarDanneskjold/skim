package builtins

import (
	"bytes"
	"errors"
	"fmt"

	"go.spiff.io/skim/lisp/interp"
	"go.spiff.io/skim/lisp/skim"
)

type Lambda struct {
	ctx      *interp.Context
	args     []skim.Symbol
	defaults []skim.Atom
	body     *skim.Cons
}

func NewLambda(ctx *interp.Context, args []skim.Symbol, body *skim.Cons) (*Lambda, error) {
	if body == nil {
		return nil, errors.New("skim: no body for lambda")
	}
	return &Lambda{
		ctx:  ctx,
		args: append([]skim.Symbol(nil), args...),
		body: skim.Dup(body).(*skim.Cons),
	}, nil
}

func (*Lambda) SkimAtom() {}

func (l *Lambda) String() string {
	if l == nil {
		return "#nil"
	}

	var buf bytes.Buffer
	buf.WriteString("(lambda [")
	for i, name := range l.args {
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(string(name))
	}
	buf.WriteString("] ")
	body := l.body.String()
	if body != "" && body[0] == '(' {
		body = body[1 : len(body)-1]
	}
	buf.WriteString(body)
	buf.WriteByte(')')

	return buf.String()
}

func (l *Lambda) GoString() string {
	return fmt.Sprintf("#<procedure %p %v>", l, l)
}

func (l *Lambda) Eval(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	var (
		args  = l.args
		nargs = len(args)
		argi  = 0
		ok    bool
		call  = l.ctx.Overlay(ctx)
	)

	for ; form != nil; argi++ {
		if argi >= nargs {
			return nil, errors.New("skim: too many arguments to lambda")
		}

		arg, err := ctx.Fork().Eval(form.Car)
		if err != nil {
			return nil, fmt.Errorf("skim: error evaluating argument #%d: %v", argi+1, err)
		}

		call.Bind(args[argi], arg)
		if form.Cdr == nil {
			argi++
			break
		} else if form, ok = form.Cdr.(*skim.Cons); !ok {
			return nil, errors.New("skim: arguments do not form a list")
		}
	}
	if argi != nargs {
		return nil, fmt.Errorf("skim: too few arguments to lambda; got %d, expected %d", argi, nargs)
	}

	err = skim.Walk(l.body, func(a skim.Atom) (err error) {
		result, err = call.Eval(a)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

var errLambdaForm = errors.New("skim: lambda must be of the form (lambda [args...] body...)")

func newLambda(ctx *interp.Context, form *skim.Cons) (skim.Atom, error) {
	if form == nil {
		return nil, errLambdaForm
	}

	body, bodyok := form.Cdr.(*skim.Cons)

	var (
		argsym []skim.Symbol
		syms   map[skim.Symbol]struct{}
	)
	args, ok := form.Car.(skim.Vector)
	if !ok {
		body = form
		goto construct
	}

	if !bodyok {
		return nil, fmt.Errorf("skim: lambda body must be a list; got %T", form.Cdr)
	}

	syms = make(map[skim.Symbol]struct{}, len(args))
	argsym = make([]skim.Symbol, len(args))
	for i, v := range args {
		if sym, ok := v.(skim.Symbol); ok {
			if _, ok = syms[sym]; ok {
				return nil, fmt.Errorf("skim: duplicate argument symbol %q", sym)
			}
			argsym[i] = sym
		} else {
			argsym, body = nil, form
			break
		}
	}

construct:
	return NewLambda(ctx, argsym, body)
}
