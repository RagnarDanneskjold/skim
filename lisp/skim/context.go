package skim

import (
	"errors"
	"fmt"
)

type Context struct {
	up    *Context
	table map[Symbol]Atom

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

func (c *Context) Bind(name Symbol, value Atom) *Context {
	c.table[name] = value
	return c
}

func (c *Context) BindProc(name Symbol, proc Proc) *Context {
	c.table[name] = proc
	return c
}

func (c *Context) Unbind(name Symbol) bool {
	_, ok := c.table[name]
	if ok {
		delete(c.table, name)
	}
	return ok
}

func (c *Context) Resolve(name Symbol) (value Atom, ok bool) {
	for ; c != nil; c = c.up {
		if value, ok = c.table[name]; ok {
			return
		}
	}
	return nil, false
}

func (c *Context) Fork() *Context {
	return &Context{
		up:    c,
		table: make(map[Symbol]Atom),
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

func (c *Context) Eval(a Atom) (result Atom, err error) {
	switch a := a.(type) {
	case *Cons:
		var proc Proc
		switch car := a.Car.(type) {
		case Symbol:
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

		var argv *Cons
		var ok bool
		if a.Cdr == nil {
			// niladic procedure call (proc has to determine if this is valid)
		} else if argv, ok = a.Cdr.(*Cons); !ok {
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

	case Proc:
		return a, nil
	case Bool:
		return a, nil
	case Int:
		return a, nil
	case Float:
		return a, nil
	case String:
		return a, nil
	case Symbol:
		v, ok := c.Resolve(a)
		if !ok {
			return nil, fmt.Errorf("undefined symbol: %v", a)
		}
		return v, nil
	}

	return nil, fmt.Errorf("unsupported execution atom: %T", a)
}
