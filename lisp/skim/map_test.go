package skim

import (
	"errors"
	"reflect"
	"testing"
)

func TestMap(t *testing.T) {
	type testCase struct {
		name    string
		in      Atom
		want    Atom
		wanterr error
		fn      MapFunc
	}

	var requireNoCall MapFunc = func(Atom) (Atom, error) {
		return nil, errors.New("map was called")
	}

	var addOne MapFunc = func(a Atom) (Atom, error) {
		return a.(Int) + 1, nil
	}

	cases := []testCase{
		{
			name:    "nil",
			in:      nil,
			want:    nil,
			wanterr: nil,
			fn:      requireNoCall,
		},

		// cons
		{
			name:    "cons/mapfn-error",
			in:      List(Int(1), Int(2), Int(3)).(*Cons),
			want:    nil,
			wanterr: errors.New("map was called"),
			fn:      requireNoCall,
		},
		{
			name:    "cons/mapfn-nil",
			in:      List(Int(1), Int(2), Int(3)).(*Cons),
			want:    nil,
			wanterr: errors.New("skim: MapFunc is nil"),
			fn:      nil,
		},
		{
			name:    "cons/nil-add-1",
			in:      nil,
			want:    nil,
			wanterr: nil,
			fn:      addOne,
		},
		{
			name:    "cons/mapfn-add-1",
			in:      List(Int(1), Int(2), Int(3)).(*Cons),
			want:    List(Int(2), Int(3), Int(4)).(*Cons),
			wanterr: nil,
			fn:      addOne,
		},

		// vector
		{
			name:    "vector/mapfn-error",
			in:      Vector{Int(1), Int(2), Int(3)},
			want:    nil,
			wanterr: errors.New("map was called"),
			fn:      requireNoCall,
		},
		{
			name:    "vector/mapfn-nil",
			in:      Vector{Int(1), Int(2), Int(3)},
			want:    nil,
			wanterr: errors.New("skim: MapFunc is nil"),
			fn:      nil,
		},
		{
			name:    "vector/nil-add-1",
			in:      nil,
			want:    nil,
			wanterr: nil,
			fn:      addOne,
		},
		{
			name:    "vector/mapfn-add-1",
			in:      Vector{Int(1), Int(2), Int(3)},
			want:    Vector{Int(2), Int(3), Int(4)},
			wanterr: nil,
			fn:      addOne,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			switch got, goterr := Map(c.in, c.fn); {
			case (goterr == nil) != (c.wanterr == nil) || (goterr != nil && goterr.Error() != c.wanterr.Error()):
				// Do not perform a deep comparison on errors since the error from
				// map is mostly arbitrary.
				t.Fatalf("Map( %v ) err = %v; want %v", c.in, goterr, c.wanterr)

			case !reflect.DeepEqual(got, c.want):
				t.Fatalf("Map( %v ) = %v; want %v", c.in, got, c.want)
			}
		})
	}
}
