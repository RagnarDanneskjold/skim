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
	builtins.BindArithmetic(ctx)
	builtins.BindMutative(ctx)
	first := true
	skim.Walk(roots, func(a skim.Atom) error {
		if !first {
			fmt.Println("")
		}
		first = false
		fmt.Printf("; %#v\n%v\n", a, a)
		v, err := ctx.Eval(a)
		var next interface{} = v
		if err != nil {
			next = err
		}
		fmt.Printf("; => %v\n; [D] => %#v\n", next, next)
		return nil
	})
}
