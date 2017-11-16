## unusedargs

`unusuedargs` finds functions and methods that have unused receivers or parameters.

__Install:__ `go get github.com/nishanths/unusedargs`

__Usage:__ `unusedargs -h`

### Example

```
func authURL(clientID, code int, state string) string {
   return fmt.Sprintf("https://example.org/?client_id=%d&code=%d", clientID, code)
}

$ unusedargs
/home/growl/go/src/code.org/x/main.go:8:1: authURL has unused param state
```

### But I need the parameter to satisfy an interface

Yes, sometimes there are legitimate reasons to have an unused parameter, such as when
implementing an interface. For example, the `Write` method neither uses its receiver nor
its parameter.

```
// BlackHole is an io.Writer that discards everything written to it
// without error.
type BlackHole struct{}

func (b *BlackHole) Write(p []byte) (int, error) {
   return 0, nil
}
```

An idiomatic way to write this would be to omit the receiver or
use the blank identifier (`_`), like so:

```
func (*BlackHole) Write(_ []byte) (int, error) {
   return 0, nil
}
```

which will make `unusedargs` no longer print a warning, and has the advanatage 
of communicating to consumers of your code that the method never uses the inputs.

### License

BSD 3-Clause.
