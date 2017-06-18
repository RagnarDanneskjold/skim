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
	newPair func() *skim.Cons
	up      *scope
	open    bool // if true, requires a closing parenthesis
	head    skim.Atom
	cdr     *skim.Atom
}

func newScope(up *scope, open bool, newPair func() *skim.Cons) *scope {
	s := new(scope)
	s.reset(up, open, newPair)
	return s
}

func (s *scope) reset(up *scope, open bool, newPair func() *skim.Cons) {
	*s = scope{
		newPair: newPair,
		up:      up,
		open:    open,
		head:    nil,
		cdr:     &s.head,
	}
}

func (s *scope) cons() skim.Atom {
	if s.head == nil {
		return s.newPair()
	}
	return s.head
}

func (s *scope) append(tip skim.Atom) {
	if v, ok := s.head.(skim.Vector); ok {
		s.head = append(v, tip)
		return
	}
	next := s.newPair()
	next.Car, *s.cdr, s.cdr = tip, next, &next.Cdr
}

// decoder is a wrapper around an io.Reader for the purpose of doing by-rune parsing of input. It
// also holds enough state to track line, column, key prefixes (from sections), and errors.
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

	pairbufSize int
	pairbufHead int
	pairbuf     []skim.Cons
}

const (
	rNewline      = '\n'
	rComment      = ';'
	rOpenParen    = '('
	rCloseParen   = ')'
	rOpenBracket  = '['
	rCloseBracket = ']'
	rString       = '"'
	rQuote        = '\''
	rBacktick     = '`'
	rComma        = ','
)

func (d *decoder) allocPair() *skim.Cons {
	sz := d.pairbufSize
	if sz == 1 {
		return new(skim.Cons)
	}

	head, buf := d.pairbufHead, d.pairbuf
	if head == len(buf) {
		head, buf = 0, make([]skim.Cons, sz)
		d.pairbuf = buf
	}
	d.pairbufHead = head + 1
	return &buf[head]
}

func (d *decoder) readSyntax() (next nextfunc, err error) {
	if err = d.skipSpace(true); err != nil {
		return nil, err
	} else if d.err != nil {
		return nil, d.err
	}

	d.buffer.Reset()
	switch d.current {
	case rOpenParen:
		return d.readList()
	case rCloseParen:
		return d.closeList()
	case rComment:
		return d.readComment()
	case rQuote, rBacktick, rComma:
		return d.readLiteral()
	case rString:
		return d.readString()
	case rOpenBracket:
		return d.readVector()
	case rCloseBracket:
		return d.closeVector()
	default:
		return d.readSymbol()
	}

	return nil, d.syntaxerr(BadCharError(d.current))
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
	err = d.readUntilBuffer(runestr(`"\`))
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
		if err != nil {
			return nil, err
		}
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
			d.buffer.WriteRune(escaped(r))
		}
		return d.readString, err
	}

	d.last.append(skim.String(d.buffer.String()))

	if err = d.skip(); err == io.EOF {
		return nil, nil
	}
	return d.readSyntax, err
}

var sentinelRunes = runestr("()[]'\",`;")

func isSymbolic(r rune) bool {
	return unicode.IsSpace(r) || sentinelRunes.Contains(r)
}

func (d *decoder) seal(force bool) (nextfunc, error) {
	for ; force || (d.last.up != nil && !d.last.open); force = false {
		a := d.last.cons()
		if a != nil {
			d.last.up.append(a)
		}
		d.last = d.last.up
	}

	return d.readSyntax, nil
}

func (d *decoder) close() (nextfunc, error) {
	if d.last.up == nil {
		return nil, d.syntaxerr(errors.New("cannot close current scope"))
	}
	return d.seal(true)
}

func (d *decoder) assign(a skim.Atom) (nextfunc, error) {
	d.last.append(a)
	return d.seal(false)
}

func (d *decoder) readSymbol() (next nextfunc, err error) {
	d.buffer.WriteRune(d.current)
	err = d.readUntilBuffer(runeFunc(isSymbolic))
	if err == io.EOF {
		err = nil // handle it next time around
	} else if err != nil {
		return nil, err
	}

	txt := d.buffer.Bytes()

	// Try numbers
	{
		var (
			n     = len(txt)
			txt   = txt
			first = txt[0]
			neg   = first == '-'
		)

		if neg || txt[0] == '+' {
			txt = txt[1:]
			if n--; n < 1 {
				goto symbol
			}
			first = txt[0]
		}

		zero := n > 0 && first == '0'
		if first == '.' {
			goto float
		} else if zero && n > 1 {
			var integer int64
			switch second := txt[1]; second {
			case 'x': // hex (16)
				if integer, err = strconv.ParseInt(string(txt[2:]), 16, 64); err == nil {
					break
				}
				goto symbol
			case '0', '1', '2', '3', '4', '5', '6', '7': // octal (8)
				if integer, err = strconv.ParseInt(string(txt[1:]), 8, 64); err == nil {
					break
				}
				goto integer
			case '8', '9':
				goto integer
			case '.':
				goto float
			default:
				goto symbol
			}

			if neg {
				integer = -integer
			}
			return d.assign(skim.Int(integer))
		} else if zero {
			return d.assign(skim.Int(0))
		}

	integer: // base 10
		if first < '0' || first > '9' {
			goto symbol
		}

		if integer, err := strconv.ParseInt(string(txt), 10, 64); err == nil {
			if neg {
				integer = -integer
			}
			return d.assign(skim.Int(integer))
		}

	float:
		if fp, err := strconv.ParseFloat(string(txt), 64); err == nil {
			if neg {
				fp = -fp
			}
			return d.assign(skim.Float(fp))
		}
	}

symbol:
	var a skim.Atom
	if n := len(txt); txt[0] == '#' && n > 1 {
		switch second := txt[1]; {
		case n == 2 && (second == 't' || second == 'f'):
			a = skim.Bool(second == 't')
		case n == 4 && second == 'n':
			if txt[2] == 'i' && txt[3] == 'l' {
				a = nil
				break
			}
			fallthrough
		default:
			a = skim.Symbol(txt)
		}
	} else if n > 3 && d.current == '\n' && txt[2] == '<' && txt[1] == '<' && txt[0] == '<' {
		// HEREDOC
		end := make([]byte, n-3)
		copy(end, txt[3:])
		d.buffer.Reset()

		for {
			err = d.readUntilBuffer(runeFunc(isSymbolic))
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

	return d.assign(a)
}

func (d *decoder) closeVector() (next nextfunc, err error) {
	if _, ok := d.last.head.(skim.Vector); !ok || !d.last.open {
		return nil, d.syntaxerr(BadCharError(']'))
	}

	err = d.skip()
	if err == io.EOF {
		err = nil
	} else if err != nil {
		return nil, err
	}

	return d.close()
}

func (d *decoder) closeList() (next nextfunc, err error) {
	if _, ok := d.last.head.(*skim.Cons); (!ok && d.last.head != nil) || !d.last.open {
		return nil, d.syntaxerr(BadCharError(')'))
	}

	err = d.skip()
	if err == io.EOF {
		err = nil
	} else if err != nil {
		return nil, err
	}

	return d.close()
}

func (d *decoder) unimplemented() (nextfunc, error) {
	return nil, errors.New("unimplemented")
}

func (d *decoder) readList() (next nextfunc, err error) {
	d.push(scopeBraced)
	return d.readSyntax, d.skip()
}

func (d *decoder) readVector() (next nextfunc, err error) {
	d.push(scopeBraced)
	d.last.head = skim.Vector{}
	return d.readSyntax, d.skip()
}

func (d *decoder) push(open bool) *scope {
	s := newScope(d.last, open, d.allocPair)
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
	if err = d.readUntilBuffer(oneRune(rNewline)); err == io.EOF {
		return nil, nil
	}
	return d.readSyntax, err
}

func (d *decoder) reset(r io.Reader) {
	const (
		defaultPairbufSize = 16
		defaultBufferCap   = 64
	)

	d.root.reset(nil, false, d.allocPair)
	d.root.head = skim.Vector(nil)
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

	if d.pairbufSize <= 0 {
		d.pairbufSize = defaultPairbufSize
	}
	d.pairbufHead, d.pairbuf = 0, nil
}

func Read(r io.Reader) (skim.Vector, error) {
	var dec decoder
	return dec.Read(r)
}

func (d *decoder) Read(r io.Reader) (skim.Vector, error) {
	d.reset(r)
	if err := d.read(); err != nil {
		return nil, err
	}
	root := d.root.cons()
	d.root, d.last = scope{head: skim.Vector(nil)}, &d.root
	d.buffer.Reset()
	d.pairbufHead, d.pairbuf = 0, nil

	return root.(skim.Vector), nil
}

func (d *decoder) read() (err error) {
	defer func() {
		rc := recover()
		if perr, ok := rc.(error); ok {
			err = perr
		} else if rc != nil {
			err = fmt.Errorf("skim: panic: %v", rc)
		}
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

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

func (d *decoder) skipSpace(newlines bool) (err error) {
	fn := unicode.IsSpace
	if !newlines {
		fn = isHorizSpace
	}

	if !fn(d.current) {
		return nil
	}

	var r rune
	for {
		r, _, err = d.nextRune()
		if err != nil {
			return err
		} else if !fn(r) {
			return nil
		}
	}
	return err
}

func (d *decoder) nextRune() (r rune, size int, err error) {
	if d.err != nil {
		return 0, 1, d.err
	}

	if d.readrune != nil {
		r, size, err = d.readrune()
	} else { // slow fallback
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

func (d *decoder) readUntilBuffer(oneof runeset) (err error) {
	var r rune
	for out := &d.buffer; ; {
		r, _, err = d.nextRune()
		if err != nil {
			return err
		} else if oneof.Contains(r) {
			return nil
		}
		out.WriteRune(r)
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
