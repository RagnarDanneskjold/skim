package builtins

import (
	"errors"
	"fmt"
	"math"

	"go.spiff.io/skim/lisp/interp"
	"go.spiff.io/skim/lisp/skim"
)

// Binary operator functions

type binopFunc func(l, r skim.Numeric) (skim.Numeric, error)

func sum(l, r skim.Numeric) (skim.Numeric, error) {
	float := l.IsFloat() || r.IsFloat()
	if float {
		l, ok := l.Float64()
		if !ok {
			fmt.Errorf("+: unable to convert argument [1] to Float")
		}
		r, ok := r.Float64()
		if !ok {
			fmt.Errorf("+: unable to convert argument [2] to Float")
		}
		return skim.Float(l + r), nil
	}
	{
		l, ok := l.Int64()
		if !ok {
			fmt.Errorf("+: unable to convert argument [1] to Int")
		}
		r, ok := r.Int64()
		if !ok {
			fmt.Errorf("+: unable to convert argument [2] to Int")
		}
		return skim.Int(l + r), nil
	}
}

func sub(l, r skim.Numeric) (skim.Numeric, error) {
	float := l.IsFloat() || r.IsFloat()
	if float {
		l, ok := l.Float64()
		if !ok {
			fmt.Errorf("-: unable to convert argument [1] to Float")
		}
		r, ok := r.Float64()
		if !ok {
			fmt.Errorf("-: unable to convert argument [2] to Float")
		}
		return skim.Float(l - r), nil
	}
	{
		l, ok := l.Int64()
		if !ok {
			fmt.Errorf("-: unable to convert argument [1] to Int")
		}
		r, ok := r.Int64()
		if !ok {
			fmt.Errorf("-: unable to convert argument [2] to Int")
		}
		return skim.Int(l - r), nil
	}
}

func mul(l, r skim.Numeric) (skim.Numeric, error) {
	float := l.IsFloat() || r.IsFloat()
	if float {
		l, ok := l.Float64()
		if !ok {
			fmt.Errorf("*: unable to convert argument [1] to Float")
		}
		r, ok := r.Float64()
		if !ok {
			fmt.Errorf("*: unable to convert argument [2] to Float")
		}
		return skim.Float(l * r), nil
	}
	{
		l, ok := l.Int64()
		if !ok {
			fmt.Errorf("*: unable to convert argument [1] to Int")
		}
		r, ok := r.Int64()
		if !ok {
			fmt.Errorf("*: unable to convert argument [2] to Int")
		}
		return skim.Int(l * r), nil
	}
}

func div(l, r skim.Numeric) (skim.Numeric, error) {
	float := l.IsFloat() || r.IsFloat()
	if float {
		l, ok := l.Float64()
		if !ok {
			fmt.Errorf("/: unable to convert argument [1] to Float")
		}
		r, ok := r.Float64()
		if !ok {
			fmt.Errorf("/: unable to convert argument [2] to Float")
		}
		if r == 0 {
			return nil, errors.New("attempt to divide by zero")
		}
		return skim.Float(l / r), nil
	}
	{
		l, ok := l.Int64()
		if !ok {
			fmt.Errorf("/: unable to convert argument [1] to Int")
		}
		r, ok := r.Int64()
		if !ok {
			fmt.Errorf("/: unable to convert argument [2] to Int")
		}
		if r == 0 {
			return nil, errors.New("attempt to divide by zero")
		}
		return skim.Int(l / r), nil
	}
}

func binopReduce(name, verb string, opfn binopFunc, nargs int) interp.Proc {
	return func(ctx *interp.Context, argv *skim.Cons) (result skim.Atom, err error) {
		if argv == nil {
			return nil, fmt.Errorf("%s: expected >=%d arguments; got 0", name, nargs)
		}
		memo, _ := argv.Car.(skim.Numeric)
		if memo == nil {
			return nil, fmt.Errorf("%s: cannot %s a %T atom", name, verb, argv.Car)
		}
		argc := 1
		err = skim.Walk(argv.Cdr, func(a skim.Atom) error {
			argc++
			n, _ := a.(skim.Numeric)
			if n == nil {
				return fmt.Errorf("%s: cannot %s a %T atom", name, verb, a)
			}
			memo, err = opfn(memo, n)
			return err
		})
		if err != nil {
			return nil, err
		}
		if argc < nargs {
			return nil, fmt.Errorf("%s: expected >=%d arguments; got %d", name, nargs, argc)
		}
		return memo, nil
	}
}

var (
	sumOp = binopReduce("+", "sum", sum, 1)
	mulOp = binopReduce("*", "multiply", mul, 1)
	subOp = binopReduce("-", "subtract", sub, 1)
	divOp = binopReduce("/", "divide", div, 2)
)

func Sum(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return skim.Int(0), nil
	}

	if form, err = Expand(ctx, form); err != nil {
		return nil, err
	}

	return sumOp(ctx, form)
}

func Mul(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return skim.Int(1), nil
	}

	if form, err = Expand(ctx, form); err != nil {
		return nil, err
	}

	return mulOp(ctx, form)
}

func Sub(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form == nil {
		return nil, errors.New("-: expected >=1 arguments; got 0")
	}

	if form, err = Expand(ctx, form); err != nil {
		return nil, err
	}

	if form.Cdr == nil {
		rhs, ok := form.Car.(skim.Numeric)
		if !ok {
			return nil, fmt.Errorf("-: cannot negate a %T atom", form.Car)
		}
		return sub(skim.Int(0), rhs)
	}
	return subOp(ctx, form)
}

func Div(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form, err = Expand(ctx, form); err != nil {
		return nil, err
	}

	return divOp(ctx, form)
}

func Mod(ctx *interp.Context, form *skim.Cons) (result skim.Atom, err error) {
	if form, err = Expand(ctx, form); err != nil {
		return nil, err
	}

	l, r, err := skim.Pair(form)
	if err != nil {
		return nil, fmt.Errorf("modulo: expected 2 arguments")
	}

	lhs, ok := l.(skim.Numeric)
	if !ok {
		return nil, errors.New("modulo: [1] Numeric expected")
	}

	rhs, ok := r.(skim.Numeric)
	if !ok {
		return nil, errors.New("modulo: [2] Numeric expected")
	}

	if lhs.IsFloat() || rhs.IsFloat() {
		lhs, ok := lhs.Float64()
		if !ok {
			return nil, fmt.Errorf("modulo: [1] cannot convert to Float")
		}
		rhs, ok := rhs.Float64()
		if !ok {
			return nil, fmt.Errorf("modulo: [2] cannot convert to Float")
		}
		return skim.Float(math.Mod(lhs, rhs)), nil
	}

	if lhs, ok := lhs.Int64(); !ok {
		return nil, fmt.Errorf("modulo: [1] cannot convert to Int")
	} else if rhs, ok := rhs.Int64(); !ok {
		return nil, fmt.Errorf("modulo: [2] cannot convert to Int")
	} else {
		return skim.Int(lhs % rhs), nil
	}
}

func BindArithmetic(ctx *interp.Context) {
	ctx.BindProc("+", Sum)
	ctx.BindProc("-", Sub)
	ctx.BindProc("*", Mul)
	ctx.BindProc("/", Div)
	ctx.BindProc("modulo", Mod)
}
