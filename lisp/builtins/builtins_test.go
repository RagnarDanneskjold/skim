package builtins

import (
	"errors"
	"reflect"
	"testing"

	"go.spiff.io/skim/lisp/skim"
)

func TestMap(t *testing.T) {
	type testCase struct {
		name    string
		in      *skim.Cons
		want    *skim.Cons
		wanterr error
		fn      MapFunc
	}

	var requireNoCall MapFunc = func(skim.Atom) (skim.Atom, error) {
		return nil, errors.New("map was called")
	}

	var addOne MapFunc = func(a skim.Atom) (skim.Atom, error) {
		return a.(skim.Int) + 1, nil
	}

	cases := []testCase{
		{
			name:    "nil",
			in:      nil,
			want:    nil,
			wanterr: nil,
			fn:      requireNoCall,
		},
		{
			name:    "mapfn-error",
			in:      skim.List(skim.Int(1), skim.Int(2), skim.Int(3)).(*skim.Cons),
			want:    nil,
			wanterr: errors.New("map was called"),
			fn:      requireNoCall,
		},
		{
			name:    "mapfn-nil",
			in:      skim.List(skim.Int(1), skim.Int(2), skim.Int(3)).(*skim.Cons),
			want:    nil,
			wanterr: errors.New("map: MapFunc is nil"),
			fn:      nil,
		},
		{
			name:    "nil-add-1",
			in:      nil,
			want:    nil,
			wanterr: nil,
			fn:      addOne,
		},
		{
			name:    "mapfn-add-1",
			in:      skim.List(skim.Int(1), skim.Int(2), skim.Int(3)).(*skim.Cons),
			want:    skim.List(skim.Int(2), skim.Int(3), skim.Int(4)).(*skim.Cons),
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
