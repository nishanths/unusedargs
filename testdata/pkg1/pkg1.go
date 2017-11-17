package pkg1

type T int

func BlankParam(_ string)       {}
func UnnamedParam(string)       {}
func (_ *T) BlankRecv(_ string) {}
func (*T) UnnamedRecv(_ string) {}

func VarArgsUnused(x, y int, s ...string) { _, _ = x, y }
func VarArgsUsed(x, y int, s ...string)   { _, _, _ = x, y, s }
func RegularArgsUnused(x, y int)          { _ = x }

func NakedReturnUnused(x int) (y int) {
	return
}

func FuncLiteral() {
	go func(x int) {
	}(42)
	_ = func(y int) {
	}
}

func ScopeUnused(n string) {
	{
		var n int
		println(n)
	}
	{
		var n string
		println(n)
	}
}

func ScopeUsed(n string) {
	{
		var n int
		println(n)
	}
	{
		var n string
		println(n)
	}
	println(n)
}
