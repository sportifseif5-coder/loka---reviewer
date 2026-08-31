package vetbroken

import "fmt"

// Greet returns a greeting; the format verb is deliberately wrong so go vet
// reports a diagnostic for the analyzer fixture.
func Greet(name string) string {
	fmt.Printf("%d", name)
	return "hello " + name
}
