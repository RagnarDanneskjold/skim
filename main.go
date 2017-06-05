package main

import (
	"fmt"
	"log"
	"os"

	"go.spiff.io/skim/internal/debug"
	"go.spiff.io/skim/lisp/builtins"
	"go.spiff.io/skim/lisp/interp"
	"go.spiff.io/skim/lisp/parser"
	"go.spiff.io/skim/lisp/skim"
)

func main() {
	log.SetFlags(0)
	debug.SetLogger(log.Print)
	roots, err := parser.Read(os.Stdin)
	if err != nil {
		log.Fatal("decode: ", err)
	}

	ctx := interp.NewContext()
	builtins.BindCore(ctx)
	builtins.BindDisplay(ctx)
	skim.Walk(roots, func(a skim.Atom) error {
		pre := fmt.Sprintf("; %#v\n", a)
		post := fmt.Sprintf("%v ; => ", a)
		v, err := ctx.Eval(a)
		if err != nil {
			post += fmt.Sprintf("ERR: %v\n", err)
		} else {
			post += fmt.Sprintf("%v\n", v)
		}

		fmt.Print(pre, post)
		fmt.Println("")

		return nil
	})
}
