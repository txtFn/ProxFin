package src

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func startupTestFunction(
	t *testing.T,
	filename string,
	name string,
) *ast.FuncDecl {

	t.Helper()

	file, err := parser.ParseFile(
		token.NewFileSet(),
		filename,
		nil,
		0,
	)

	if err != nil {
		t.Fatal(err)
	}

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)

		if ok && function.Name.Name == name {
			return function
		}
	}

	t.Fatalf("Function not found: %s", name)
	return nil
}

func startupTestCalls(
	function *ast.FuncDecl,
	name string,
) int {

	count := 0

	ast.Inspect(function.Body, func(node ast.Node) bool {

		call, ok := node.(*ast.CallExpr)

		if !ok {
			return true
		}

		identifier, ok := call.Fun.(*ast.Ident)

		if ok && identifier.Name == name {
			count++
		}

		return true
	})

	return count
}

func TestStartupOwnershipPublicEntry(t *testing.T) {

	function := startupTestFunction(
		t,
		"config.go",
		"StartSystem",
	)

	if startupTestCalls(function, "runRefreshTransaction") != 1 {
		t.Fatal("Startup must acquire one transaction")
	}

	if startupTestCalls(function, "startSystemLocked") != 1 {
		t.Fatal("Startup must call its internal implementation")
	}
}

func TestStartupOwnershipInternalEntry(t *testing.T) {

	function := startupTestFunction(
		t,
		"config.go",
		"startSystemLocked",
	)

	for _, name := range []string{
		"StartSystem",
		"runRefreshTransaction",
	} {
		if startupTestCalls(function, name) != 0 {
			t.Fatalf("Unexpected nested call: %s", name)
		}
	}
}

func TestStartupOwnershipSynchronousXEPG(t *testing.T) {

	function := startupTestFunction(
		t,
		"config.go",
		"startSystemLocked",
	)

	count := 0

	ast.Inspect(function.Body, func(node ast.Node) bool {

		call, ok := node.(*ast.CallExpr)

		if !ok {
			return true
		}

		identifier, ok := call.Fun.(*ast.Ident)

		if !ok || identifier.Name != "buildXEPG" {
			return true
		}

		count++

		if len(call.Args) != 1 {
			t.Fatal("Unexpected XEPG arguments")
		}

		argument, ok := call.Args[0].(*ast.Ident)

		if !ok || argument.Name != "false" {
			t.Fatal("Startup XEPG must be synchronous")
		}

		return true
	})

	if count != 1 {
		t.Fatalf("Expected one XEPG call, got %d", count)
	}
}

func TestStartupOwnershipPublicRestore(t *testing.T) {

	function := startupTestFunction(
		t,
		"backup.go",
		"ThreadfinRestore",
	)

	if startupTestCalls(function, "runRefreshTransaction") != 1 {
		t.Fatal("Public restore must acquire one transaction")
	}

	if startupTestCalls(function, "threadfinRestoreLocked") != 1 {
		t.Fatal("Public restore must call its internal implementation")
	}
}

func TestStartupOwnershipInternalRestore(t *testing.T) {

	function := startupTestFunction(
		t,
		"backup.go",
		"threadfinRestoreLocked",
	)

	if startupTestCalls(function, "startSystemLocked") != 1 {
		t.Fatal("Restore must reuse startup lock ownership")
	}

	for _, name := range []string{
		"StartSystem",
		"ThreadfinRestore",
		"runRefreshTransaction",
	} {
		if startupTestCalls(function, name) != 0 {
			t.Fatalf("Recursive transaction risk: %s", name)
		}
	}
}

func TestStartupOwnershipPublicWebRestore(t *testing.T) {

	function := startupTestFunction(
		t,
		"backup.go",
		"ThreadfinRestoreFromWeb",
	)

	if startupTestCalls(function, "runRefreshTransaction") != 1 {
		t.Fatal("Public web restore must acquire one transaction")
	}

	if startupTestCalls(function, "threadfinRestoreFromWebLocked") != 1 {
		t.Fatal("Missing internal web restoration call")
	}
}

func TestStartupOwnershipInternalWebRestore(t *testing.T) {

	function := startupTestFunction(
		t,
		"backup.go",
		"threadfinRestoreFromWebLocked",
	)

	if startupTestCalls(function, "threadfinRestoreLocked") != 1 {
		t.Fatal("Web restore must reuse existing lock ownership")
	}

	for _, name := range []string{
		"ThreadfinRestore",
		"ThreadfinRestoreFromWeb",
		"runRefreshTransaction",
	} {
		if startupTestCalls(function, name) != 0 {
			t.Fatalf("Recursive transaction risk: %s", name)
		}
	}
}

func TestStartupOwnershipWebSocketDispatch(t *testing.T) {

	file, err := parser.ParseFile(
		token.NewFileSet(),
		"webserver.go",
		nil,
		0,
	)

	if err != nil {
		t.Fatal(err)
	}

	lockedCalls := 0
	publicCalls := 0

	ast.Inspect(file, func(node ast.Node) bool {

		call, ok := node.(*ast.CallExpr)

		if !ok {
			return true
		}

		identifier, ok := call.Fun.(*ast.Ident)

		if !ok {
			return true
		}

		switch identifier.Name {
		case "threadfinRestoreFromWebLocked":
			lockedCalls++
		case "ThreadfinRestoreFromWeb":
			publicCalls++
		}

		return true
	})

	if lockedCalls != 1 || publicCalls != 0 {
		t.Fatalf(
			"Unexpected restore dispatch: locked=%d public=%d",
			lockedCalls,
			publicCalls,
		)
	}
}
