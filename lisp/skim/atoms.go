package skim

import (
	"bytes"
	"fmt"
	"strconv"
)

// Atom defines any value understood to be a member of a skim list, including lists themselves.
type Atom interface {
	// SkimAtom is an empty method -- it exists only to mark a type as an Atom at compile time.
	SkimAtom()
	String() string
}

type goStringer interface {
	GoString() string
}

func fmtgostring(v interface{}) string {
	switch v := v.(type) {
	case goStringer:
		return v.GoString()
	case fmt.Stringer:
		return v.String()
	case nil:
		return "#nil"
	default:
		return fmt.Sprint(v)
	}
}

func fmtstring(v interface{}) string {
	switch v := v.(type) {
	case fmt.Stringer:
		return v.String()
	case nil:
		return "#nil"
	default:
		return fmt.Sprint(v)
	}
}

// The set of runtime atoms

type Int int64

func (Int) SkimAtom()        {}
func (i Int) String() string { return strconv.FormatInt(int64(i), 10) }

type Float float64

func (Float) SkimAtom()        {}
func (f Float) String() string { return strconv.FormatFloat(float64(f), 'f', -1, 64) }

type Symbol string

const (
	noQuote    = Symbol("")
	Quote      = Symbol("quote")
	Quasiquote = Symbol("quasiquote")
	Unquote    = Symbol("unquote")
)

func (Symbol) SkimAtom() {}

func (s Symbol) String() string   { return string(s) }
func (s Symbol) GoString() string { return string(s) }

type Cons struct{ Car, Cdr Atom }

func IsNil(a Atom) bool {
	if a == nil {
		return true
	}
	switch a := a.(type) {
	case *Cons:
		return a == nil || (a.Cdr == nil && a.Car == nil)
	default:
		return false
	}
}

func (*Cons) SkimAtom() {}
func (c *Cons) string(gostring bool) string {
	if c == nil {
		return "#null"
	}

	if IsNil(c) {
		return "()"
	}

	fmtfn := fmtstring
	if gostring {
		fmtfn = fmtgostring
	} else if !gostring {
		quo := "'"
		switch c.Car {
		case Quote:
		case Unquote:
			quo = ","
		case Quasiquote:
			quo = "`"
		default:
			goto list
		}

		if c, ok := c.Cdr.(*Cons); ok {
			if IsNil(c) {
				return quo + "()"
			}

			switch c.Cdr.(type) {
			case *Cons:
				return quo + fmtstring(c)
			case nil:
				return quo + fmtstring(c.Car)
			}
		}
	}

list:
	var b bytes.Buffer
	ch := byte('(')
	for c := Atom(c); c != nil; {
		b.WriteByte(ch)
		ch = ' '

		cons, ok := c.(*Cons)
		if !ok {
			b.WriteString(". ")
			b.WriteString(fmtfn(c))
			break
		}

		b.WriteString(fmtfn(cons.Car))
		c = cons.Cdr
	}
	b.WriteByte(')')
	return b.String()
}

func (c *Cons) String() string { return c.string(false) }

func (c *Cons) GoString() string {
	if c == nil {
		return "#null"
	}
	return "(" + fmtgostring(c.Car) + " . " + fmtgostring(c.Cdr) + ")"
}

type String string

func (String) SkimAtom()          {}
func (s String) String() string   { return s.GoString() }
func (s String) GoString() string { return strconv.QuoteToASCII(string(s)) }

type Bool bool

func (Bool) SkimAtom() {}
func (b Bool) String() string {
	if b {
		return "#t"
	}
	return "#f"
}

func Pair(a Atom) (lhs, rhs Atom, err error) {
	la, ok := a.(*Cons)
	if !ok {
		return nil, nil, fmt.Errorf("atom %v is not a cons cell", a)
	}
	ra, ok := la.Cdr.(*Cons)
	if !ok {
		return nil, nil, fmt.Errorf("atom %v is not a cons cell", la.Cdr)
	}
	if ra.Cdr != nil {
		return nil, nil, fmt.Errorf("atom %v is not a pair", a)
	}
	return la.Car, ra.Car, nil
}

type Visitor func(Atom) (Visitor, error)

// Traverse will recursively visit all cons pairs and left and right elements, in order. Traversal
// ends when a visitor returns a nil visitor for nested elements and all adjacent and upper elements
// are traversed.
func Traverse(a Atom, visitor Visitor) (err error) {
traverseCdr:
	if IsNil(a) {
		return nil
	}

	visitor, err = visitor(a)
	if err != nil {
		return err
	} else if visitor == nil {
		return nil
	}

	cons, _ := a.(*Cons)
	if cons == nil {
		return nil
	}

	if !IsNil(cons.Car) {
		err = Traverse(cons.Car, visitor)
		if err != nil {
			return nil
		}
	}

	a = cons.Cdr
	goto traverseCdr
}

// Walk recursively visits all cons pairs in a singly-linked list, calling fn for the car of each
// cons pair and walking through each cdr it encounters a nil cdr. If a cdr is encountered that is
// neither a cons pair nor nil, Walk returns an error.
func Walk(a Atom, fn func(Atom) error) error {
	for {
		switch cons := a.(type) {
		case nil:
			return nil
		case *Cons:
			if cons.Car == nil && cons.Cdr == nil {
				// nil / sentinel cons
				return nil
			}

			if err := fn(cons.Car); err != nil {
				return err
			}
			a = cons.Cdr
		default:
			return fmt.Errorf("cannot walk %T", a)
		}
	}
}

func List(args ...Atom) Atom {
	if len(args) == 0 {
		return &Cons{}
	}
	cons := make([]Cons, len(args)+1)
	for i, q := range args {
		c := &cons[i]
		c.Car = q
		if i < len(args)-1 {
			c.Cdr = &cons[i+1]
		}
	}
	return &cons[0]
}

func cadr(a Atom, seq string) (Atom, error) {
	var c *Cons
	var op byte
	for i := len(seq) - 1; i >= 0; i-- {
		op = seq[i]
		c, _ = a.(*Cons)
		if c == nil {
			return nil, fmt.Errorf("c%cr: %T is not a Cons", op, a)
		} else if op == 'a' {
			a = c.Car
		} else {
			a = c.Cdr
		}
	}
	return a, nil
}

func Car(a Atom) (Atom, error) {
	c, _ := a.(*Cons)
	if c == nil {
		return nil, fmt.Errorf("car: %T is not a Cons", a)
	}
	return c.Car, nil
}

func Cdr(a Atom) (Atom, error) {
	c, _ := a.(*Cons)
	if c == nil {
		return nil, fmt.Errorf("cdr: %T is not a Cons", a)
	}
	return c.Cdr, nil
}

func Caar(a Atom) (Atom, error)   { return cadr(a, "aa") }
func Cadr(a Atom) (Atom, error)   { return cadr(a, "ad") }
func Cdar(a Atom) (Atom, error)   { return cadr(a, "da") }
func Cddr(a Atom) (Atom, error)   { return cadr(a, "dd") }
func Caaar(a Atom) (Atom, error)  { return cadr(a, "aaa") }
func Caadr(a Atom) (Atom, error)  { return cadr(a, "aad") }
func Cadar(a Atom) (Atom, error)  { return cadr(a, "ada") }
func Caddr(a Atom) (Atom, error)  { return cadr(a, "add") }
func Cdaar(a Atom) (Atom, error)  { return cadr(a, "daa") }
func Cdadr(a Atom) (Atom, error)  { return cadr(a, "dad") }
func Cddar(a Atom) (Atom, error)  { return cadr(a, "dda") }
func Cdddr(a Atom) (Atom, error)  { return cadr(a, "ddd") }
func Caaaar(a Atom) (Atom, error) { return cadr(a, "aaaa") }
func Caaadr(a Atom) (Atom, error) { return cadr(a, "aaad") }
func Caadar(a Atom) (Atom, error) { return cadr(a, "aada") }
func Caaddr(a Atom) (Atom, error) { return cadr(a, "aadd") }
func Cadaar(a Atom) (Atom, error) { return cadr(a, "adaa") }
func Cadadr(a Atom) (Atom, error) { return cadr(a, "adad") }
func Caddar(a Atom) (Atom, error) { return cadr(a, "adda") }
func Cadddr(a Atom) (Atom, error) { return cadr(a, "addd") }
func Cdaaar(a Atom) (Atom, error) { return cadr(a, "daaa") }
func Cdaadr(a Atom) (Atom, error) { return cadr(a, "daad") }
func Cdadar(a Atom) (Atom, error) { return cadr(a, "dada") }
func Cdaddr(a Atom) (Atom, error) { return cadr(a, "dadd") }
func Cddaar(a Atom) (Atom, error) { return cadr(a, "ddaa") }
func Cddadr(a Atom) (Atom, error) { return cadr(a, "ddad") }
func Cdddar(a Atom) (Atom, error) { return cadr(a, "ddda") }
func Cddddr(a Atom) (Atom, error) { return cadr(a, "dddd") }
