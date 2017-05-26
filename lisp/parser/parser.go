package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.spiff.io/skim/lisp/skim"
)

// nextfunc is a parsing function that modifies the decoder's state and returns another parsing
// function. If nextfunc returns io.EOF, parsing is complete. Any other error halts parsing.
type nextfunc func() (nextfunc, error)

type scope struct {
	up   *scope
	open bool // if true, requires a closing parenthesis
	root *skim.Cons
	last *skim.Cons
	tail *skim.Cons
}

func newScope(up *scope, open bool) *scope {
	root := new(skim.Cons)
	s := &scope{
		up:   up,
		open: open,
		root: root,
		tail: root,
	}
	return s
}

func (s *scope) cons() *skim.Cons {
	if s.last != nil {
		s.last.Cdr = nil
	}
	return s.root
}

func (s *scope) append(tip skim.Atom) {
	if skim.IsNil(tip) && !s.open {
		tip = nil
	}
	next := new(skim.Cons)
	s.tail.Car, s.tail.Cdr = tip, next
	s.last, s.tail = s.tail, next
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

	root scope
	last *scope
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

	return nil, errors.New("unimplemented")
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

	d.last.append(skim.String(d.buffer.String()))
	return d.readSyntax, d.skip()
}

var sentinelRunes = runestr("()'\",`;")

func isSymbolic(r rune) bool {
	return unicode.IsSpace(r) || sentinelRunes.Contains(r)
}

func (d *decoder) assign(a skim.Atom, close bool, next nextfunc, err error) (nextfunc, error) {
	if err != nil {
		return nil, err
	} else if d.last.up == nil && close {
		return nil, fmt.Errorf("cannot close current scope")
	}

	if a != nil {
		d.last.append(a)
	}

	for ; d.last.up != nil && d.last.open == close; close = false {
		d.last.up.append(d.last.cons())
		d.last = d.last.up
	}

	return next, nil
}

func (d *decoder) readSymbol() (next nextfunc, err error) {
	d.buffer.WriteRune(d.current)
	err = d.readUntil(runeFunc(isSymbolic), true, nil)
	if err != nil {
		return nil, err
	}

	txt := d.buffer.String()
	n := len(txt)

	// Try numbers
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
			return d.assign(skim.Int(integer), false, d.readSyntax, nil)
		}
	} else if zero { // literal zero
		return d.assign(skim.Int(0), true, d.readSyntax, nil)
	}

next:
	integer, err := strconv.ParseInt(txt, 10, 64) // decimal (10)
	if err == nil {
		return d.assign(skim.Int(integer), false, d.readSyntax, nil)
	}

	// float (10)
	fp, err := strconv.ParseFloat(txt, 64)
	if err == nil {
		d.last.append(skim.Float(fp))
		return d.assign(skim.Float(fp), false, d.readSyntax, nil)
	}

	var a skim.Atom
	if strings.HasPrefix(txt, "#") {
		switch txt {
		case "#t", "#f":
			a = skim.Bool(txt == "#t")
		default:
			return nil, d.syntaxerr(fmt.Errorf("invalid token", txt))
		}
	} else if strings.HasPrefix(txt, "<<<") && d.current == '\n' && len(txt) > 3 {
		// HEREDOC
		d.buffer.Reset()
		end := []byte(txt[3:])

		for {
			err = d.readUntil(runeFunc(isSymbolic), true, nil)
			buf := d.buffer.Bytes()
			if (err == io.EOF || err == nil) && bytes.HasSuffix(buf, end) {
				buf = buf[:len(buf)-len(end)]
				if len(buf) == 0 || buf[len(buf)-1] == '\n' {
					a = skim.String(buf)
					break
				}
			} else if err != nil {
				if err == io.EOF {
					err = io.ErrUnexpectedEOF
				}
				return nil, err
			}
			d.buffer.WriteRune(d.current)
		}

	} else {
		a = skim.Symbol(txt)
	}

	return d.assign(a, false, d.readSyntax, nil)
}

func (d *decoder) closeList() (next nextfunc, err error) {
	err = d.skip()
	if !d.last.open {
		return nil, d.syntaxerr(BadCharError(')'))
	}

	if err == io.EOF {
		err = nil
	}

	return d.assign(nil, true, d.readSyntax, err)
}

func (d *decoder) unimplemented() (nextfunc, error) {
	return nil, errors.New("unimplemented")
}

func (d *decoder) readList() (next nextfunc, err error) {
	d.push(scopeBraced)
	return d.readSyntax, d.skip()
}

func (d *decoder) push(open bool) *scope {
	s := newScope(d.last, open)
	d.last = s
	return d.last
}

const scopeBraced = true
const scopeQuoted = false

func (d *decoder) readLiteral() (next nextfunc, err error) {
	sym := skim.Quote
	switch d.current {
	case rBacktick:
		sym = skim.Quasiquote
	case rComma:
		sym = skim.Unquote
	}

	// ok:
	d.push(scopeQuoted)
	d.last.append(sym)
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

	d.root = *newScope(nil, false)
	d.last = &d.root

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

func Read(r io.Reader) (*skim.Cons, error) {
	var dec decoder
	return dec.Read(r)
}

func (d *decoder) Read(r io.Reader) (*skim.Cons, error) {
	d.reset(r)
	if err := d.read(); err != nil {
		return nil, err
	}
	root := d.root.root
	d.root, d.last = scope{}, &d.root
	d.buffer.Reset()

	return root, nil
}

func (d *decoder) read() (err error) {
	defer panictoerr(&err)
	var next nextfunc = d.start
	for next != nil && err == nil {
		next, err = next()
	}
	if err == io.EOF && d.last == &d.root {
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
