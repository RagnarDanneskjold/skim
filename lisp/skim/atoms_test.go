package skim

import (
	"reflect"
	"testing"
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
			got := List(c.Args...)
			if !reflect.DeepEqual(got, c.Want) {
				t.Fatalf("List(%v) = %v; want %v", c.Args, got, c.Want)
			}
		})
	}
}
