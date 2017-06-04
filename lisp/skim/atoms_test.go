package skim

import (
	"errors"
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

	carErr := errors.New("car: skim.Int is not a Cons")
	cdrErr := errors.New("cdr: skim.Int is not a Cons")
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

		"CarError":  {fn: Car, in: Int(1), err: carErr},
		"CdrError":  {fn: Cdr, in: Int(1), err: cdrErr},
		"CadrError": {fn: Cadr, in: &Cons{Int(1), Int(2)}, err: carErr},
		"CddrError": {fn: Cddr, in: &Cons{Int(1), Int(2)}, err: cdrErr},
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

func BenchmarkCadrs(b *testing.B) {
	seq := List(Int(0), Int(0), Int(0), Int(0))
	seq = List(seq, seq, seq, seq, seq)
	seq = List(seq, seq, seq, seq, seq)
	seq = List(seq, seq, seq, seq, seq)

	cases := map[string]func(Atom) (Atom, error){
		"Car":    Car,
		"Cdr":    Cdr,
		"Caar":   Caar,
		"Cadr":   Cadr,
		"Cdar":   Cdar,
		"Cddr":   Cddr,
		"Caaar":  Caaar,
		"Caadr":  Caadr,
		"Cadar":  Cadar,
		"Caddr":  Caddr,
		"Cdaar":  Cdaar,
		"Cdadr":  Cdadr,
		"Cddar":  Cddar,
		"Cdddr":  Cdddr,
		"Caaaar": Caaaar,
		"Caaadr": Caaadr,
		"Caadar": Caadar,
		"Caaddr": Caaddr,
		"Cadaar": Cadaar,
		"Cadadr": Cadadr,
		"Caddar": Caddar,
		"Cadddr": Cadddr,
		"Cdaaar": Cdaaar,
		"Cdaadr": Cdaadr,
		"Cdadar": Cdadar,
		"Cdaddr": Cdaddr,
		"Cddaar": Cddaar,
		"Cddadr": Cddadr,
		"Cdddar": Cdddar,
		"Cddddr": Cddddr,
	}

	b.ResetTimer()

	for name, fn := range cases {
		b.Run(name, func(b *testing.B) {
			for i := b.N; i > 0; i-- {
				if a, err := fn(seq); a == nil || err != nil {
					b.FailNow()
				}
			}
		})
	}
}
