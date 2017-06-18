package skim

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// Atom defines any value understood to be a member of a skim list, including lists themselves.
type Atom interface {
	// SkimAtom is an empty method -- it exists only to mark a type as an Atom at compile time.
	SkimAtom()
	String() string
}

type Numeric interface {
	Atom

	IsFloat() bool
	Int64() (int64, bool)
	Float64() (float64, bool)
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

func (Int) SkimAtom()                  {}
func (i Int) String() string           { return strconv.FormatInt(int64(i), 10) }
func (Int) IsFloat() bool              { return false }
func (i Int) Float64() (float64, bool) { return float64(i), true }
func (i Int) Int64() (int64, bool)     { return int64(i), true }

type Float float64

func (Float) SkimAtom()                  {}
func (f Float) String() string           { return strconv.FormatFloat(float64(f), 'f', -1, 64) }
func (Float) IsFloat() bool              { return true }
func (f Float) Float64() (float64, bool) { return float64(f), true }
func (f Float) Int64() (int64, bool)     { return int64(f), true }

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

func IsTrue(a Atom) bool {
	switch a := a.(type) {
	case Bool:
		return bool(a)
	case nil:
		return false
	case *Cons:
		return a != nil && (a.Car != nil || a.Cdr != nil)
	}
	return true
}

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

func (c *Cons) Map(fn MapFunc) (result Atom, err error) {
	if c == nil { // typed nil - distinct from Atom(nil)
		return nil, nil
	}

	n := 1
	for counter := c; counter.Cdr != nil; {
		n++
		if counter.Cdr == nil {
			break
		} else if next, ok := counter.Cdr.(*Cons); ok {
			counter = next
		} else {
			return nil, fmt.Errorf("skim: map: cannot map a list with a Cdr of %T", counter.Cdr)
		}
	}

	var (
		mapped = make([]Cons, n)
		pred   = &result
	)
	for i := range mapped {
		mpair := &mapped[i]
		if mpair.Car, err = fn.Map(c.Car); err != nil {
			return nil, err
		}
		*pred, pred = mpair, &mpair.Cdr
		c, _ = c.Cdr.(*Cons)
	}

	return result, nil
}

type Vector []Atom

func (Vector) SkimAtom()          {}
func (v Vector) String() string   { return v.format(fmtstring) }
func (v Vector) GoString() string { return v.format(fmtgostring) }

func (v Vector) format(format func(interface{}) string) string {
	vs := "["
	for i, a := range v {
		if i > 0 {
			vs += " "
		}
		vs += format(a)
	}
	vs += "]"
	return vs
}

func (v Vector) Map(fn MapFunc) (result Atom, err error) {
	if v == nil { // typed nil - distinct from Atom(nil)
		return Vector(nil), nil
	}

	mapped := make(Vector, len(v))
	for i, a := range v {
		if mapped[i], err = fn.Map(a); err != nil {
			return nil, err
		}
	}
	return mapped, nil
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
	if !ok || la == nil {
		return nil, nil, errors.New("skim: (car atom) is not a *Cons")
	}
	ra, ok := la.Cdr.(*Cons)
	if !ok || ra == nil {
		return nil, nil, errors.New("skim: (cdr atom) is not a *Cons")
	} else if ra.Cdr != nil {
		return nil, nil, errors.New("skim: (cdr atom) is not a *Cons of the form (a . (b . #nil))")
	}
	return la.Car, ra.Car, nil
}

type Visitor func(Atom) (Visitor, error)

// Traverse will recursively visit all cons pairs and left and right elements, in order. Traversal
// ends when a visitor returns a nil visitor for nested elements and all adjacent and upper elements
// are traversed. If a Vector is encountered, the vector itself is passed to the visitor function
// followed by its elements (passed to the visitor returned for the Vector).
func Traverse(a Atom, visitor Visitor) (err error) {
traverseCdr:
	if IsNil(a) {
		return nil
	}

	if vec, ok := a.(Vector); ok {
		visitor, err = visitor(a)
		if visitor == nil || err != nil {
			return err
		}

		for _, a := range vec {
			if err = Traverse(a, visitor); err != nil {
				return err
			}
		}
		return nil
	}

	visitor, err = visitor(a)
	if visitor == nil || err != nil {
		return err
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
// neither a cons pair nor nil, Walk returns an error. If the atom, a, is a Vector, it will call fn
// for each element of the vector.
func Walk(a Atom, fn func(Atom) error) error {
	if vec, ok := a.(Vector); ok {
		for _, elem := range vec {
			if err := fn(elem); err != nil {
				return err
			}
		}
		return nil
	}

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
			return fmt.Errorf("skim: cannot walk %T", a)
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
			return nil, fmt.Errorf("skim: c%cr: %T is not a *Cons", op, a)
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
		return nil, fmt.Errorf("skim: car: %T is not a *Cons", a)
	}
	return c.Car, nil
}

func Cdr(a Atom) (Atom, error) {
	c, _ := a.(*Cons)
	if c == nil {
		return nil, fmt.Errorf("skim: cdr: %T is not a *Cons", a)
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

// MapFunc is a function used to map an atom to another atom. It may return an error, in which case
// the caller should assume the result atom is unusable unless documented otherwise for special uses
// of a specific MapFunc.
type MapFunc func(Atom) (Atom, error)

func (fn MapFunc) Map(atom Atom) (Atom, error) {
	if fn == nil {
		return nil, errors.New("skim: MapFunc is nil")
	}
	return fn(atom)
}

type Mapper interface {
	Map(MapFunc) (Atom, error)
}

// Map iterates over a list and maps its values using mapfn. It returns a new list with the mapped
// values. The input list must be, strictly, a list -- that is, all Cdrs of the input list must
// either be nil or another cons cell meeting the same criteria.
func Map(list Atom, mapfn MapFunc) (result Atom, err error) {
	if list == nil {
		return nil, nil
	}

	m, ok := list.(Mapper)
	if !ok {
		return nil, fmt.Errorf("skim: cannot map %T; does not implement Mapper")
	}
	return m.Map(mapfn)
}
