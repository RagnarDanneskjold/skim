package interp

import (
	"errors"
	"fmt"
	"sync"

	"go.spiff.io/skim/lisp/skim"
)

type Evaler interface {
	skim.Atom

	Eval(*Context, *skim.Cons) (skim.Atom, error)
}

type unbound struct{}

func (unbound) SkimAtom()      {}
func (unbound) String() string { return "#unbound" }

var Unbound = unbound{}

type Context struct {
	up *Context

	// table is the set of values bound to symbols in this scope and descendant scopes.
	table map[skim.Symbol]skim.Atom // inherited
	tm    sync.RWMutex

	// upval is the table of upvalues names to opaque values (empty interfaces). These are used
	// as private data held by the current context, in the event that there is shared
	// information for the context. Contexts are not permitted to access parent contexts'
	// upvalues. Unlike tables, assigning a nil value to an upvalue deletes the upvalue -- as
	// such, contexts do not inherit each others' upvalues.
	upval map[string]interface{}
	um    sync.RWMutex
}

func NewContext() *Context {
	return (*Context).Fork(nil)
}

// Dup clones a context, flattening it into a single Context of known bindings and c's upvalues.
func (c *Context) Dup() *Context {
	base := NewContext()
	{ // Copy upper-most upvalues
		table := base.upval
		for k, v := range c.upval {
			table[k] = v
		}
	}
	for table := base.table; c != nil; c = c.up {
		for k, v := range c.table {
			if v == Unbound {
				continue
			} else if _, set := table[k]; !set {
				table[k] = v
			}
		}
	}
	return base
}

func (c *Context) Fork() *Context {
	return &Context{
		up:    c,
		table: make(map[skim.Symbol]skim.Atom),
		upval: make(map[string]interface{}),
	}
}

func (c *Context) Overlay(parent *Context) *Context {
	c = c.Dup()
	c.up = parent
	return c
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
	if c == nil {
		return nil
	}
	c.um.RLock()
	defer c.um.RUnlock()
	return c.upval[name]
}

func (c *Context) Bind(name skim.Symbol, value skim.Atom) *Context {
	if c == nil {
		return nil
	}
	c.tm.Lock()
	defer c.tm.Unlock()
	c.table[name] = value
	return c
}

func (c *Context) BindProc(name skim.Symbol, proc Proc) *Context {
	return c.Bind(name, proc)
}

func (c *Context) Unbind(name skim.Symbol) (ok bool) {
	if c == nil {
		return false
	}

	c.tm.Lock()
	defer c.tm.Unlock()
	if _, ok = c.table[name]; ok {
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
		c.tm.RLock()
		value, bound, ok = resolveInTable(name, c.table)
		if bound {
			c.tm.RUnlock()
			return
		}
		c.tm.RUnlock()
	}
	return nil, false
}

func (c *Context) Parent() *Context {
	if c == nil {
		return nil
	}
	return c.up
}

func (c *Context) Eval(a skim.Atom) (result skim.Atom, err error) {
	switch a := a.(type) {
	case *skim.Cons:
		if a == nil {
			return nil, nil
		}

		var proc skim.Atom
		proc, err = c.Eval(a.Car)
		if err != nil {
			return nil, err
		}

		evaler, ok := proc.(Evaler)
		if !ok {
			return nil, fmt.Errorf("skim: cannot call type %T", proc)
		}

		var argv *skim.Cons
		if a.Cdr == nil {
			// niladic procedure call (proc has to determine if this is valid)
		} else if argv, ok = a.Cdr.(*skim.Cons); !ok {
			return nil, errors.New("skim: ill-formed procedure call")
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

		return evaler.Eval(c, argv)

	case skim.Symbol:
		v, ok := c.Resolve(a)
		if !ok {
			return nil, fmt.Errorf("skim: undefined symbol: %v", a)
		}
		return v, nil
	}

	return a, nil
}
