package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func main() {
	var dec decoder
	dec.reset(os.Stdin)
	if err := dec.read(); err != nil {
		log.Fatal("decode: ", err)
	}

	root := dec.root.Atom.(*List).Atoms
	for _, a := range root {
		fmt.Printf("%#v\n", a)
	}

	log.Print("done")
}

// Atom defines any value understood to be a member of a skim list, including lists themselves.
type Atom interface {
	// SkimAtom is an empty method -- it exists only to mark a type as an Atom at compile time.
	SkimAtom()
	String() string
}

type goStringer interface {
	GoString() string
}

type Int int64

func (Int) SkimAtom()          {}
func (i Int) String() string   { return strconv.FormatInt(int64(i), 10) }
func (i Int) GoString() string { return "int{" + i.String() + "}" }

type Float float64

func (Float) SkimAtom()          {}
func (f Float) String() string   { return strconv.FormatFloat(float64(f), 'f', -1, 64) }
func (f Float) GoString() string { return "float{" + f.String() + "}" }

type Symbol string

func (Symbol) SkimAtom() {}

func (s Symbol) String() string   { return string(s) }
func (s Symbol) GoString() string { return "sym{" + s.String() + "}" }

type List struct{ Atoms []Atom }

func (l *List) String() string {
	return l.string(false)
}

func (l *List) GoString() string {
	return l.string(true)
}

func (l *List) string(gostring bool) string {
	if l == nil {
		return "nil"
	} else if len(l.Atoms) == 0 {
		return "()"
	}

	// Render quoted list forms specially
	if gostring {
		// nop
	} else if sym, ok := l.Atoms[0].(Symbol); ok && len(l.Atoms) == 2 {
		switch sym {
		case Symbol("quote"):
			return "'" + l.Atoms[1].String()
		case Symbol("quasiquote"):
			return "`" + l.Atoms[1].String()
		case Symbol("unquote"):
			return "," + l.Atoms[1].String()
		}
	}

	descs := make([]string, len(l.Atoms))
	for i, v := range l.Atoms {
		if !gostring {
		} else if gv, ok := v.(goStringer); ok {
			descs[i] = gv.GoString()
			continue
		}
		descs[i] = v.String()
	}
	return "(" + strings.Join(descs, " ") + ")"
}

func (List) SkimAtom() {}

type QuoteKind rune

const (
	QLiteral    QuoteKind = '\''
	QQuasiquote QuoteKind = '`'
	QUnquote    QuoteKind = ','
)

func (q QuoteKind) symbol() Symbol {
	switch q {
	case QLiteral:
		return Symbol("quote")
	case QQuasiquote:
		return Symbol("quasiquote")
	case QUnquote:
		return Symbol("unquote")
	}
	return Symbol("#error:bad-quote/" + strconv.Itoa(int(q)))
}

type Quote struct {
	Kind QuoteKind
	Atom
}

func (l *Quote) getAtom() Atom {
	if l == nil {
		return nil
	}
	return l.Atom
}

func (l *Quote) String() string {
	if l.getAtom() == nil {
		return "'#error:nil"
	}
	return string(l.Kind) + l.Atom.String()
}
func (l *Quote) GoString() string {
	if a, ok := l.getAtom().(goStringer); ok {
		return string(l.Kind) + a.GoString()
	}
	return l.String()
}

func (*Quote) SkimAtom() {}

type String string

func (String) SkimAtom()          {}
func (s String) String() string   { return string(s) }
func (s String) GoString() string { return strconv.QuoteToASCII(s.String()) }

// nextfunc is a parsing function that modifies the decoder's state and returns another parsing
// function. If nextfunc returns io.EOF, parsing is complete. Any other error halts parsing.
type nextfunc func() (nextfunc, error)

type scope struct {
	up *scope
	Atom
}

func (s *scope) append(tip Atom) error {
	switch atom := s.Atom.(type) {
	case *List:
		atom.Atoms = append(atom.Atoms, tip)
	case *Quote:
		atom.Atom = tip
	default:
		return fmt.Errorf("skim: attempt to add atom to non-list type %T", atom)
	}
	return nil
}

func (s *scope) ascend() (parentScope, containerScope *scope) {
	if s == nil || s.up == nil {
		return s, nil
	}
	// Never hop more than one literal up
	if _, ok := s.up.Atom.(*Quote); ok {
		log.Print("return grandparent: ", s.up.up, "; grandchild: ", s)
		return s.up.up, s.up
	}
	log.Print("return parent: ", s.up, "; child: ", s)
	return s.up, s.up
}

// decoder is a wrapper around an io.Reader for the purpose of doing by-rune parsing of INI file
// input. It also holds enough state to track line, column, key prefixes (from sections), and
// errors.
type decoder struct {
	rd       io.Reader
	readrune func() (rune, int, error)

	err       error
	current   rune
	line, col int

	// Storage
	buffer bytes.Buffer
	key    string

	// peek / next state
	havenext bool
	next     rune
	nexterr  error

	last, root *scope
}

const (
	rNewline    = '\n'
	rComment    = ';'
	rOpenParen  = '('
	rCloseParen = ')'
	rString     = '"'
	rQuote      = '\''
	rBacktick   = '`'
	rComma      = ','
)

func (d *decoder) readSyntax() (next nextfunc, err error) {
	if err = must(d.skipSpace(true), io.EOF); err == io.EOF {
		return nil, err
	}

	if d.err != nil {
		return nil, err
	}

	d.buffer.Reset()
	switch d.current {
	case rOpenParen:
		return d.readList, nil
	case rCloseParen:
		return d.closeList, nil
	case rComment:
		return d.readComment, nil
	case rQuote, rBacktick, rComma: // quote
		return d.readLiteral, nil
	case rString:
		return d.readString, nil
	default:
		return d.readSymbol, nil
	}

	log.Printf("%q -> unimplemented", d.current)
	return nil, errors.New("unimplemented")
}

func isQuote(a Atom, kind QuoteKind) bool {
	switch a := a.(type) {
	case *Quote:
		return a.Kind == kind
	case *List:
		return len(a.Atoms) > 0 && a.Atoms[0] == kind.symbol()
	}
	return false
}

func (d *decoder) inQuote(kind QuoteKind) bool {
	for up := d.last; up != nil && up != d.root; up = up.up {
		if isQuote(up.Atom, kind) {
			return true
		}
	}
	return false
}

func (d *decoder) readHexCode(size int) (result rune, err error) {
	for i := 0; i < size; i++ {
		r, sz, err := d.nextRune()
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return -1, d.syntaxerr(err, "expected hex code")
		} else if sz != 1 {
			// Quick size check
			return -1, d.syntaxerr(BadCharError(r), "expected hex code")
		}

		if r >= 'A' && r <= 'F' {
			r = 10 + (r - 'A')
		} else if r >= 'a' && r <= 'f' {
			r = 10 + (r - 'a')
		} else if r >= '0' && r <= '9' {
			r -= '0'
		} else {
			return -1, d.syntaxerr(BadCharError(r), "expected hex code")
		}
		result = result<<4 | r
	}
	return result, nil
}

func (d *decoder) readString() (next nextfunc, err error) {
	err = d.readUntil(runestr(`"\`), true, nil)
	if err == io.EOF {
		return nil, d.syntaxerr(UnclosedError('"'), "encountered EOF inside string")
	} else if err != nil {
		return nil, err
	}

	switch d.current {
	case '"':
		log.Print("end of string")
		// done

	case '\\':
		r, _, err := d.nextRune()
		must(err)
		switch r {
		case 'x': // 1 octet
			r, err = d.readHexCode(2)
			d.buffer.WriteByte(byte(r & 0xFF))
		case 'u': // 2 octets
			r, err = d.readHexCode(4)
			d.buffer.WriteRune(r)
		case 'U': // 4 octets
			r, err = d.readHexCode(8)
			d.buffer.WriteRune(r)
		default:
			r = escaped(r)
			d.buffer.WriteRune(escaped(r))
		}
		return d.readString, err
	}

	defer stopOnEOF(&next, &err)
	return d.claimAtom(String(d.buffer.String()), d.readSyntax, d.skip())
}

var sentinelRunes = runestr("()'\",`")

func isSymbolic(r rune) bool {
	return unicode.IsSpace(r) || sentinelRunes.Contains(r)
}

func (d *decoder) readSymbol() (next nextfunc, err error) {
	d.buffer.WriteRune(d.current)
	err = d.readUntil(runeFunc(isSymbolic), true, nil)
	if err != nil {
		return nil, err
	}

	txt := d.buffer.String()
	log.Printf("numeric: %q", txt)
	n := len(txt)

	zero := n > 0 && txt[0] == '0'
	if zero && n > 1 {
		var integer int64
		if txt[1] == 'x' { // hex (16)
			integer, err = strconv.ParseInt(txt[2:], 16, 64)
		} else if txt[1] >= '0' && txt[1] <= 7 { // octal (8)
			integer, err = strconv.ParseInt(txt[1:], 8, 64)
		} else {
			goto next
		}

		if err == nil {
			return d.claimAtom(Int(integer), d.readSyntax, nil)
		}
	} else if zero { // literal zero
		return d.claimAtom(Int(0), d.readSyntax, nil)
	}

next:
	integer, err := strconv.ParseInt(txt, 10, 64) // decimal (10)
	if err == nil {
		return d.claimAtom(Int(integer), d.readSyntax, nil)
	}

	// float (10)
	fp, err := strconv.ParseFloat(txt, 64)
	if err == nil {
		return d.claimAtom(Float(fp), d.readSyntax, nil)
	}

	return d.claimAtom(Symbol(txt), d.readSyntax, nil)
}

func (d *decoder) claimAtom(a Atom, next nextfunc, err error) (nextfunc, error) {
	d.last = &scope{up: d.last, Atom: a}
	return d.ascend(d.assign(next, err))
}

func (d *decoder) closeList() (next nextfunc, err error) {
	if _, ok := d.last.Atom.(*List); !ok {
		return nil, d.syntaxerr(errors.New("unexpected ')'"))
	}

	err = d.skip()
	if up, _ := d.last.ascend(); up == d.root && err == io.EOF {
		err = nil
	}
	return d.ascend(d.assign(d.readSyntax, err))
}

func (d *decoder) unimplemented() (nextfunc, error) {
	return nil, errors.New("unimplemented")
}

func (d *decoder) readList() (next nextfunc, err error) {
	d.last = &scope{up: d.last, Atom: &List{}}
	return d.readSyntax, d.skip()
}

func (d *decoder) ascend(next nextfunc, err error) (nextfunc, error) {
	if err != nil {
		return nil, err
	}

	up, _ := d.last.ascend()
	if up == d.last {
		return nil, fmt.Errorf("attempt to ascend scope without parent scope")
	}
	d.last = up
	return next, nil
}

func (d *decoder) assign(next nextfunc, err error) (nextfunc, error) {
	if err != nil {
		return nil, err
	}

	s := d.last
	up, container := s.ascend()
	if up == s || container == s {
		return nil, fmt.Errorf("attempt to assign atom without parent scope")
	}
	log.Printf("%#+v", container)
	if err := container.append(s.Atom); err != nil {
		return nil, err
	}
	if a, ok := container.Atom.(*Quote); ok {
		up.append(&List{Atoms: []Atom{a.Kind.symbol(), s.Atom}})
	}
	return next, nil
}

func (d *decoder) readLiteral() (next nextfunc, err error) {
	if d.current == rComma {
		for up := d.last; up != nil; up = up.up {
			switch a := up.Atom.(type) {
			case *List:
				if len(a.Atoms) >= 1 && a.Atoms[0] == Symbol("quasiquote") {
					goto ok
				}

			case *Quote:
				if a.Kind == QQuasiquote {
					goto ok
				}
			}
		}
		return nil, d.syntaxerr(ErrUnquoteContext)
	}

ok:
	lit := &Quote{Kind: QuoteKind(d.current)}
	d.last = &scope{up: d.last, Atom: lit}

	return d.readSyntax, d.skip()
}

func (d *decoder) start() (next nextfunc, err error) {
	_, _, err = d.nextRune()
	if err == io.EOF {
		return nil, nil
	}
	return d.readSyntax, err
}

func (d *decoder) readComment() (next nextfunc, err error) {
	defer stopOnEOF(&next, &err)
	return d.readSyntax, d.readUntil(oneRune(rNewline), true, nil)
}

func (d *decoder) reset(r io.Reader) {
	const defaultBufferCap = 64

	d.root = &scope{Atom: &List{}}
	d.last = d.root

	if rx, ok := r.(runeReader); ok {
		d.readrune = rx.ReadRune
	} else {
		d.readrune = nil
	}

	d.rd = r
	d.err = nil

	d.current = 0
	d.line = 1
	d.col = 0

	d.buffer.Reset()
	d.buffer.Grow(defaultBufferCap)

	d.havenext = false
	d.nexterr = nil
}

func (d *decoder) read() (err error) {
	defer panictoerr(&err)
	var next nextfunc = d.start
	for next != nil && err == nil {
		next, err = next()
	}
	if err == io.EOF && d.root == d.last {
		err = nil
	}
	return err
}

func (d *decoder) syntaxerr(err error, msg ...interface{}) *SyntaxError {
	if se, ok := err.(*SyntaxError); ok {
		return se
	}
	se := &SyntaxError{Line: d.line, Col: d.col, Err: err, Desc: fmt.Sprint(msg...)}
	return se
}

func isHorizSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\r' }

func (d *decoder) skipSpace(newlines bool) error {
	fn := unicode.IsSpace
	if !newlines {
		fn = isHorizSpace
	}

	if fn(d.current) {
		return d.readUntil(notRune(runeFunc(fn)), false, nil)
	}
	return nil
}

func (d *decoder) nextRune() (r rune, size int, err error) {
	if d.err != nil {
		return d.current, utf8.RuneLen(d.current), d.err
	}

	if d.havenext {
		r, size, err = d.peekRune()
		d.havenext = false
	} else if d.readrune != nil {
		r, size, err = d.readrune()
	} else {
		r, size, err = readrune(d.rd)
	}

	d.current = r

	if err != nil {
		d.err = err
		d.rd = nil
	}

	if d.current == '\n' {
		d.line++
		d.col = 1
	}

	return r, size, err
}

func (d *decoder) skip() error {
	_, _, err := d.nextRune()
	return err
}

func (d *decoder) peekRune() (r rune, size int, err error) {
	if d.havenext {
		r = d.next
		size = utf8.RuneLen(r)
		return r, size, d.nexterr
	}

	// Even if there's an error.
	d.havenext = true
	if d.readrune != nil {
		r, size, err = d.readrune()
	} else {
		r, size, err = readrune(d.rd)
	}
	d.next, d.nexterr = r, err
	return r, size, err
}

func (d *decoder) readUntil(oneof runeset, buffer bool, runemap func(rune) rune) (err error) {
	for out := &d.buffer; ; {
		var r rune
		r, _, err = d.nextRune()
		if err != nil {
			return err
		} else if oneof.Contains(r) {
			return nil
		} else if buffer {
			if runemap != nil {
				r = runemap(r)
			}
			if r >= 0 {
				out.WriteRune(r)
			}
		}
	}
}

func must(err error, allowed ...error) error {
	if err == nil {
		return err
	}

	for _, e := range allowed {
		if e == err {
			return err
		}
	}

	panic(err)
}

func stopOnEOF(next *nextfunc, err *error) {
	if *err == io.EOF {
		*next = nil
		*err = nil
	}
}

// Recover from unexpected errors
func panictoerr(err *error) {
	rc := recover()
	if perr, ok := rc.(error); ok {
		*err = perr
	} else if rc != nil {
		*err = fmt.Errorf("skim: panic: %v", rc)
	}

	if *err == io.EOF {
		*err = io.ErrUnexpectedEOF
	}
}

// Rune handling

type runeReader interface {
	ReadRune() (rune, int, error)
}

func readrune(rd io.Reader) (r rune, size int, err error) {
	if rd, ok := rd.(runeReader); ok {
		return rd.ReadRune()
	}
	var b [4]byte
	for i, t := 0, 1; i < len(b); i, t = i+1, t+1 {
		_, err = rd.Read(b[i:t])
		if err != nil {
			return r, size, err
		} else if c := b[:t]; utf8.FullRune(c) {
			r, size = utf8.DecodeRune(c)
			return r, size, err
		}
	}

	return unicode.ReplacementChar, 1, nil
}

type (
	runeset interface {
		Contains(rune) bool
	}

	oneRune  rune
	runeFunc func(rune) bool
	runestr  string
)

func notRune(runes runeset) runeset {
	return runeFunc(func(r rune) bool { return !runes.Contains(r) })
}

func (s runestr) Contains(r rune) bool { return strings.ContainsRune(string(s), r) }

func (fn runeFunc) Contains(r rune) bool { return fn(r) }

func (lhs oneRune) Contains(rhs rune) bool { return rune(lhs) == rhs }

func escaped(r rune) rune {
	switch r {
	case '0':
		return 0
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	default:
		return r
	}
}
