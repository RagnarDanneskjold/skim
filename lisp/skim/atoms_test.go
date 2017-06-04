package skim

import (
	"reflect"
	"testing"

	"go.spiff.io/skim/internal/debug"
)

func TestList(t *testing.T) {
	type testcase struct {
		Args []Atom
		Want Atom
	}
	cases := []testcase{
		{
			Args: nil,
			Want: &Cons{},
		},
		{
			Args: []Atom{Int(1)},
			Want: &Cons{Int(1), nil},
		},
		{
			Args: []Atom{Int(1), Bool(false)},
			Want: &Cons{Int(1), &Cons{Bool(false), nil}},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.Want.String(), func(t *testing.T) {
			debug.SetLoggerf(t.Logf)
			got := List(c.Args...)
			if !reflect.DeepEqual(got, c.Want) {
				t.Fatalf("List(%v) = %v; want %v", c.Args, got, c.Want)
			}
		})
	}
}

func TestCadr(t *testing.T) {
	seq := List(Int(1), Int(2), Int(3), Int(4), Int(5))
	nestl1 := List(List(Int(1)), List(Int(2)), List(Int(3)), List(Int(4)))
	nestr1 := &Cons{List(Int(1), Int(2), Int(3), Int(4)), Int(5)}

	type testCase struct {
		fn   func(Atom) (Atom, error)
		in   Atom
		want Atom
		err  error
	}

	cases := map[string]testCase{
		// If the following work, the rest should follow
		"Car":    {fn: Car, in: nestl1, want: List(Int(1))},
		"Cadr":   {fn: Cadr, in: nestl1, want: List(Int(2))},
		"Caddr":  {fn: Caddr, in: nestl1, want: List(Int(3))},
		"Cadddr": {fn: Cadddr, in: nestl1, want: List(Int(4))},

		"Cdr":    {fn: Cdr, in: nestr1, want: Int(5)},
		"Cdar":   {fn: Cdar, in: nestr1, want: List(Int(2), Int(3), Int(4))},
		"Cddar":  {fn: Cddar, in: nestr1, want: List(Int(3), Int(4))},
		"Cdddar": {fn: Cdddar, in: nestr1, want: List(Int(4))},

		"Cddr":   {fn: Cddr, in: seq, want: List(Int(3), Int(4), Int(5))},
		"Cdddr":  {fn: Cdddr, in: seq, want: List(Int(4), Int(5))},
		"Cddddr": {fn: Cddddr, in: seq, want: List(Int(5))},

		"CarError":  {fn: Car, in: Int(1), err: &CadrError{op: "car", typ: reflect.TypeOf(Int(0))}},
		"CdrError":  {fn: Cdr, in: Int(1), err: &CadrError{op: "cdr", typ: reflect.TypeOf(Int(0))}},
		"CadrError": {fn: Cadr, in: &Cons{Int(1), Int(2)}, err: &CadrError{op: "car", typ: reflect.TypeOf(Int(0))}},
		"CddrError": {fn: Cddr, in: &Cons{Int(1), Int(2)}, err: &CadrError{op: "cdr", typ: reflect.TypeOf(Int(0))}},
	}

	for name, c := range cases {
		name, c := name, c
		t.Run(name, func(t *testing.T) {
			debug.SetLoggerf(t.Logf)
			got, err := c.fn(c.in)
			switch {
			case !reflect.DeepEqual(c.err, err):
				t.Fatalf("%s(..) err = %v; want %v", name, err, c.err)
			case c.err != nil && err != nil && c.err.Error() != err.Error():
				t.Fatalf("%s(..) err = %v; want %v", name, err, c.err)
			case !reflect.DeepEqual(c.want, got):
				t.Fatalf("%s(..) = %v; want %v", name, got, c.want)
			}
		})
	}
}
