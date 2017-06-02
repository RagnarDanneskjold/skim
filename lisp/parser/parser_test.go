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
		in  string
		out skim.Atom
	}

	cases := map[string]testcase{
		"empty": {
			in:  "",
			out: cons(nil, nil),
		},
		"newlines": {
			in:  "\n\n\n",
			out: cons(nil, nil),
		},
		"nil": {
			in:  "#nil",
			out: skim.List(nil),
		},
		"nil/multi": {
			in:  "'#nil (#nil #nil #nil)",
			out: skim.List(quote(nil), skim.List(nil, nil, nil)),
		},
		"booleans": {
			in:  `#t #f`,
			out: skim.List(skim.Bool(true), skim.Bool(false)),
		},
		"string/empty": {
			in:  `""`,
			out: skim.List(skim.String("")),
		},
		"string/normal": {
			in:  `"foobar"`,
			out: skim.List(skim.String("foobar")),
		},
		"string/escapes": {
			in:  `"\0\x0a\x0A\a\b\f\n\r\t\v\u0000\U00000000"`,
			out: skim.List(skim.String("\x00\n\n\a\b\f\n\r\t\v\u0000\U00000000")),
		},
		"negative/symbol": {
			in:  "-",
			out: skim.List(skim.Symbol("-")),
		},
		"negative/integer-0": {
			in:  "-0",
			out: skim.List(skim.Int(0)),
		},
		"negative/integer-0xff": {
			in:  "-0xff",
			out: skim.List(skim.Int(-255)),
		},
		"negative/integer-0654": {
			in:  "-0654",
			out: skim.List(skim.Int(-428)),
		},
		"integer-0": {
			in:  "((0))",
			out: cons(cons(cons(skim.Int(0), nil), nil), nil),
		},
		"integer-0xff": {
			in:  "0xff",
			out: skim.List(skim.Int(255)),
		},
		"integer-0654": {
			in:  "0654",
			out: skim.List(skim.Int(428)),
		},
		"integer-+0xff": {
			in:  "+0xff",
			out: skim.List(skim.Int(255)),
		},
		"integer-+0654": {
			in:  "+0654",
			out: skim.List(skim.Int(428)),
		},
		"negative/float-0.0": {
			in:  "-0.0",
			out: skim.List(skim.Float(-0.0)),
		},
		"float-0.0": {
			in:  "0.0",
			out: skim.List(skim.Float(0.0)),
		},
		"float-+0.0": {
			in:  "+0.0",
			out: skim.List(skim.Float(+0.0)),
		},
		"heredoc/lines": {
			in: `(<<<---EOF
		Foobar
		Baz
---EOF)`,
			out: skim.List(cons(skim.String("\t\tFoobar\n\t\tBaz\n"), nil)),
		},
		"heredoc/empty": {
			in: `(<<<---EOF
---EOF)`,
			out: skim.List(cons(skim.String(""), nil)),
		},
		"heredoc/empty-line": {
			in: `(<<<---EOF

---EOF)`,
			out: skim.List(cons(skim.String("\n"), nil)),
		},
		"quasiquote-to-unquote": {
			in:  "`(,())",
			out: skim.List(cons(skim.Quasiquote, cons(cons(cons(skim.Unquote, cons(cons(nil, nil), nil)), nil), nil))),
		},
		"quote/empty-list": {
			in:  `'()`,
			out: skim.List(quote(cons(nil, nil))),
		},
		"quote/quote/empty-list": {
			in:  `''()`,
			out: skim.List(quote(quote(cons(nil, nil)))),
		},
		"quote/quote/nested-empty-list": {
			in:  `''(())`,
			out: skim.List(quote(quote(cons(cons(nil, nil), nil)))),
		},
		"quote/nested-empty-list": {
			in:  `'(())`,
			out: skim.List(quote(cons(cons(nil, nil), nil))),
		},
		"quote/nested-empty-lists": {
			in:  `'(() ())`,
			out: skim.List(quote(skim.List(cons(nil, nil), cons(nil, nil)))),
		},
		"quote/empty-list-verbatim": {
			in:  `(quote ())`,
			out: skim.List(quote(cons(nil, nil))),
		},
		"quote/nested-empty-list-verbatim": {
			in:  `(quote (()))`,
			out: skim.List(quote(cons(cons(nil, nil), nil))),
		},
		"comment": {
			in:  "\n\n; a comment\n\n",
			out: skim.List(),
		},
		"comment-to-eof": {
			in:  "\n\n; a comment",
			out: skim.List(),
		},
		"let": {
			in: `(let ((name "Foo Bar")                                              ; Comment on first line
			           (age 123))                                                    ; Comment on second line
			       (display "Happy birthday, " name " for reaching age " (+ age 1))) ; Another comment until EOF`,
			out: skim.List(
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
			),
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
			if err != nil {
				t.Fatalf("Read(%q) err = %#+v; want nil", c.in, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Read(%q) failed;\ngot  %v\nwant %v", c.in, got, want)
			}
		})
		t.Run(name+"/one-byte-reader", func(t *testing.T) {
			debug.SetLoggerf(t.Logf)
			got, err := Read(iotest.OneByteReader(strings.NewReader(c.in)))
			want := c.out
			if err != nil {
				t.Fatalf("Read(%q) err = %#+v; want nil", c.in, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Read(%q) failed;\ngot  %v\nwant %v", c.in, got, want)
			}
		})
	}
}
