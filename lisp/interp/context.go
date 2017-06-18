package interp

import (
	"errors"
	"fmt"

	"go.spiff.io/skim/lisp/skim"
)

type unbound struct{}

func (unbound) SkimAtom()      {}
func (unbound) String() string { return "#unbound" }

var Unbound = unbound{}

type Context struct {
	up *Context
	// table is the set of values bound to symbols in this scope and descendant scopes.
	table map[skim.Symbol]skim.Atom // inherited

	// upval is the table of upvalues names to opaque values (empty interfaces). These are used
	// as private data held by the current context, in the event that there is shared
	// information for the context. Contexts are not permitted to access parent contexts'
	// upvalues. Unlike tables, assigning a nil value to an upvalue deletes the upvalue -- as
	// such, contexts do not inherit each others' upvalues.
	upval map[string]interface{}
}

func (c *Context) SetUpvalue(name string, val interface{}) *Context {
	if val != nil {
		c.upval[name] = val
	} else {
		delete(c.upval, name)
	}
	return c
}

func (c *Context) Upvalue(name string) interface{} {
	return c.upval[name]
}

func (c *Context) Bind(name skim.Symbol, value skim.Atom) *Context {
	c.table[name] = value
	return c
}

func (c *Context) BindProc(name skim.Symbol, proc Proc) *Context {
	c.table[name] = proc
	return c
}

func (c *Context) Unbind(name skim.Symbol) bool {
	_, ok := c.table[name]
	if ok {
		c.table[name] = Unbound
	}
	return ok
}

func resolveInTable(name skim.Symbol, table map[skim.Symbol]skim.Atom) (value skim.Atom, bound, ok bool) {
	if value, ok = table[name]; !ok {
		return value, false, ok
	}
	bound = true
	if value == Unbound { // value is occluded in this context
		value, ok = nil, false
	}
	return value, bound, ok
}

func (c *Context) Resolve(name skim.Symbol) (value skim.Atom, ok bool) {
	var bound bool
	for ; c != nil; c = c.up {
		value, bound, ok = resolveInTable(name, c.table)
		if bound {
			return
		}
	}
	return nil, false
}

func (c *Context) Fork() *Context {
	return &Context{
		up:    c,
		table: make(map[skim.Symbol]skim.Atom),
		upval: make(map[string]interface{}),
	}
}

func (c *Context) Parent() *Context {
	if c == nil {
		return nil
	}
	return c.up
}

func NewContext() *Context {
	return (*Context).Fork(nil)
}

func (c *Context) Eval(a skim.Atom) (result skim.Atom, err error) {
	switch a := a.(type) {
	case *skim.Cons:
		var proc Proc
		switch car := a.Car.(type) {
		case skim.Symbol:
			v, ok := c.Resolve(car)
			if !ok {
				return nil, fmt.Errorf("undefined symbol: %v", car)
			}
			if proc, ok = v.(Proc); !ok {
				return nil, fmt.Errorf("%s: cannot call atom of type %T", car, v)
			}

		case Proc:
			proc = car

		default:
			return nil, fmt.Errorf("cannot call atom of type: %T", a.Car)
		}

		var argv *skim.Cons
		var ok bool
		if a.Cdr == nil {
			// niladic procedure call (proc has to determine if this is valid)
		} else if argv, ok = a.Cdr.(*skim.Cons); !ok {
			return nil, errors.New("ill-formed procedure call")
		}

		defer func() {
			switch rc := recover().(type) {
			case nil:
				return
			case error:
				err = rc
			default:
				err = fmt.Errorf("PANIC: %v", rc)
			}
			result = nil
		}()

		return proc(c, argv)
	case skim.Symbol:
		v, ok := c.Resolve(a)
		if !ok {
			return nil, fmt.Errorf("undefined symbol: %v", a)
		}
		return v, nil
	case nil:
		return nil, nil
	}

	return a, nil
}
