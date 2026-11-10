//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/activityforge/activityforge/internal/browser"
)

func main() {
	evaluate := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 || args[0].Type() != js.TypeString {
			return `{"error":"expected a JSON request"}`
		}
		return browser.Evaluate(args[0].String())
	})
	js.Global().Set("activityForge", evaluate)
	select {}
}
