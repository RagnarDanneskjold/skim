package parser

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/iotest"

	"go.spiff.io/skim/internal/debug"
	"go.spiff.io/skim/lisp/skim"
)

func cons(a, b skim.Atom) skim.Atom {
	return &skim.Cons{a, b}
}

func quote(a skim.Atom) skim.Atom {
	return cons(skim.Quote, cons(a, nil))
}

func TestParse(t *testing.T) {
	type testcase struct {
		in   string
		out  skim.Atom
		fail bool
	}

	cases := map[string]testcase{
		"empty": {
			in:  "",
			out: skim.Vector(nil),
		},
		"newlines": {
			in:  "\n\n\n",
			out: skim.Vector(nil),
		},
		"nil": {
			in:  "#nil",
			out: skim.Vector{nil},
		},
		"nil/multi": {
			in:  "'#nil (#nil #nil #nil)",
			out: skim.Vector{quote(nil), skim.List(nil, nil, nil)},
		},
		"booleans": {
			in:  `#t #f`,
			out: skim.Vector{skim.Bool(true), skim.Bool(false)},
		},
		"string/empty": {
			in:  `""`,
			out: skim.Vector{skim.String("")},
		},
		"string/normal": {
			in:  `"foobar"`,
			out: skim.Vector{skim.String("foobar")},
		},
		"string/escapes": {
			in:  `"\0\x0a\x0A\a\b\f\n\r\t\v\u0000\U00000000"`,
			out: skim.Vector{skim.String("\x00\n\n\a\b\f\n\r\t\v\u0000\U00000000")},
		},
		"negative/symbol": {
			in:  "-",
			out: skim.Vector{skim.Symbol("-")},
		},
		"negative/integer-0": {
			in:  "-0",
			out: skim.Vector{skim.Int(0)},
		},
		"negative/integer-0xff": {
			in:  "-0xff",
			out: skim.Vector{skim.Int(-255)},
		},
		"negative/integer-0654": {
			in:  "-0654",
			out: skim.Vector{skim.Int(-428)},
		},
		"integer-0": {
			in:  "((0))",
			out: skim.Vector{cons(cons(skim.Int(0), nil), nil)},
		},
		"integer-0xff": {
			in:  "0xff",
			out: skim.Vector{skim.Int(255)},
		},
		"integer-0654": {
			in:  "0654",
			out: skim.Vector{skim.Int(428)},
		},
		"integer-+0xff": {
			in:  "+0xff",
			out: skim.Vector{skim.Int(255)},
		},
		"integer-+0654": {
			in:  "+0654",
			out: skim.Vector{skim.Int(428)},
		},
		"negative/float-0.0": {
			in:  "-0.0",
			out: skim.Vector{skim.Float(-0.0)},
		},
		"float-0.0": {
			in:  "0.0",
			out: skim.Vector{skim.Float(0.0)},
		},
		"float-+0.0": {
			in:  "+0.0",
			out: skim.Vector{skim.Float(+0.0)},
		},
		"symbol/hex-like": {
			in:  "0xfoobar",
			out: skim.Vector{skim.Symbol("0xfoobar")},
		},
		"symbol/#foobar": {
			in:  "#foobar",
			out: skim.Vector{skim.Symbol("#foobar")},
		},
		"heredoc/lines": {
			in: `(<<<---EOF
		Foobar
		Baz
---EOF)`,
			out: skim.Vector{cons(skim.String("\t\tFoobar\n\t\tBaz\n"), nil)},
		},
		"heredoc/empty": {
			in: `(<<<---EOF
---EOF)`,
			out: skim.Vector{cons(skim.String(""), nil)},
		},
		"heredoc/empty-line": {
			in: `(<<<---EOF

---EOF)`,
			out: skim.Vector{cons(skim.String("\n"), nil)},
		},
		"quasiquote-to-unquote": {
			in:  "`(,())",
			out: skim.Vector{cons(skim.Quasiquote, cons(cons(cons(skim.Unquote, cons(cons(nil, nil), nil)), nil), nil))},
		},
		"quote/empty-list": {
			in:  `'()`,
			out: skim.Vector{quote(cons(nil, nil))},
		},
		"quote/quote/empty-list": {
			in:  `''()`,
			out: skim.Vector{quote(quote(cons(nil, nil)))},
		},
		"quote/quote/nested-empty-list": {
			in:  `''(())`,
			out: skim.Vector{quote(quote(cons(cons(nil, nil), nil)))},
		},
		"quote/nested-empty-list": {
			in:  `'(())`,
			out: skim.Vector{quote(cons(cons(nil, nil), nil))},
		},
		"quote/nested-empty-lists": {
			in:  `'(() ())`,
			out: skim.Vector{quote(skim.List(cons(nil, nil), cons(nil, nil)))},
		},
		"quote/empty-list-verbatim": {
			in:  `(quote ())`,
			out: skim.Vector{quote(cons(nil, nil))},
		},
		"quote/nested-empty-list-verbatim": {
			in:  `(quote (()))`,
			out: skim.Vector{quote(cons(cons(nil, nil), nil))},
		},
		"comment": {
			in:  "\n\n; a comment\n\n",
			out: skim.Vector(nil),
		},
		"comment-to-eof": {
			in:  "\n\n; a comment",
			out: skim.Vector(nil),
		},
		"vector/empty": {
			in:  "[]",
			out: skim.Vector{skim.Vector{}},
		},
		"vector/nonempty": {
			in:  `[1 -2 "three"]`,
			out: skim.Vector{skim.Vector{skim.Int(1), skim.Int(-2), skim.String("three")}},
		},
		"vector/nested-in-vector": {
			in:  `[[1 -2 "three"]]`,
			out: skim.Vector{skim.Vector{skim.Vector{skim.Int(1), skim.Int(-2), skim.String("three")}}},
		},
		"vector/nested-in-list": {
			in:  `([1 -2 "three"])`,
			out: skim.Vector{skim.List(skim.Vector{skim.Int(1), skim.Int(-2), skim.String("three")})},
		},
		"let": {
			in: `(let ((name "Foo Bar")                                              ; Comment on first line
			           (age 123))                                                    ; Comment on second line
			       (display "Happy birthday, " name " for reaching age " (+ age 1))) ; Another comment until EOF`,
			out: skim.Vector{
				skim.List(
					skim.Symbol("let"),
					skim.List(
						skim.List(skim.Symbol("name"), skim.String("Foo Bar")),
						skim.List(skim.Symbol("age"), skim.Int(123)),
					),
					skim.List(skim.Symbol("display"),
						skim.String("Happy birthday, "),
						skim.Symbol("name"),
						skim.String(" for reaching age "),
						skim.List(skim.Symbol("+"), skim.Symbol("age"), skim.Int(1)),
					),
				),
			},
		},

		"error/cons/closed-by-vector": {
			in:   `(]`,
			fail: true,
		},
		"error/cons/unclosed": {
			in:   `(`,
			fail: true,
		},
		"error/cons/double-close": {
			in:   `())`,
			fail: true,
		},
		"error/vector/closed-by-list": {
			in:   `[)`,
			fail: true,
		},
		"error/vector/unclosed": {
			in:   `[`,
			fail: true,
		},
		"error/vector/double-close": {
			in:   `[]]`,
			fail: true,
		},
		"error/vector/close-root": {
			in:   `]`,
			fail: true,
		},
		"error/cons/close-root": {
			in:   `)`,
			fail: true,
		},
	}

	keys := make([]string, 0, len(cases))
	for name := range cases {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	for _, name := range keys {
		c := cases[name]
		t.Run(name+"/string-reader", func(t *testing.T) {
			debug.SetLoggerf(t.Logf)
			got, err := Read(strings.NewReader(c.in))
			want := c.out
			if match := (err != nil) != c.fail; match {
				want := "nil"
				if c.fail {
					want = "error"
				}
				t.Fatalf("Read(%q) err = (%T) %v; want %s", c.in, err, err, want)
			}
			if !c.fail && !reflect.DeepEqual(got, want) {
				t.Fatalf("Read(%q) failed;\ngot  %v\nwant %v", c.in, got, want)
			}
		})
		t.Run(name+"/one-byte-reader", func(t *testing.T) {
			debug.SetLoggerf(t.Logf)
			got, err := Read(iotest.OneByteReader(strings.NewReader(c.in)))
			want := c.out
			if match := (err != nil) != c.fail; match {
				want := "nil"
				if c.fail {
					want = "error"
				}
				t.Fatalf("Read(%q) err = (%T) %v; want %s", c.in, err, err, want)
			}
			if !c.fail && !reflect.DeepEqual(got, want) {
				t.Fatalf("Read(%q) failed;\ngot  %v\nwant %v", c.in, got, want)
			}
		})
	}
}
