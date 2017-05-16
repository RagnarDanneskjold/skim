package main

import (
	"errors"
	"fmt"
)

var ErrUnquoteContext = errors.New("use of unquote outside of quasiquote context")

// SyntaxError is an error returned when the INI parser encounters any syntax it does not
// understand. It contains the line, column, any other error encountered, and a description of the
// syntax error.
type SyntaxError struct {
	Line, Col int
	Err       error
	Desc      string
}

func (s *SyntaxError) Error() string {
	if s.Desc == "" {
		return fmt.Sprintf("skim: syntax error at %d:%d: %v", s.Line, s.Col, s.Err)
	}
	return fmt.Sprintf("skim: syntax error at %d:%d: %v -- %s", s.Line, s.Col, s.Err, s.Desc)
}

// UnclosedError is an error describing an unclosed bracket from {, (, [, and <. It is typically set
// as the Err field of a SyntaxError.
//
// Its value is expected to be one of the above opening braces.
type UnclosedError rune

// Expecting returns the rune that was expected but not found for the UnclosedError's rune value.
func (u UnclosedError) Expecting() rune {
	switch u := rune(u); u {
	case '{':
		return '}'
	case '(':
		return ')'
	case '[':
		return ']'
	case '<':
		return '>'
	default:
		return u
	}
}

func (u UnclosedError) Error() string {
	return fmt.Sprintf("skim: unclosed %c, expecting %c", rune(u), u.Expecting())
}

// BadCharError is an error describing an invalid character encountered during parsing. It is
// typically set as the Err field of a SyntaxError.
type BadCharError rune

func (r BadCharError) Error() string {
	return fmt.Sprintf("skim: encountered invalid character %q", rune(r))
}
