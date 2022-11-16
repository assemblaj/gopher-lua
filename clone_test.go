package lua

/*
   Broadly speaking, these tests are the regular tests, but instead swapping a clone in after performing
   certain functions and seeing if they return the same result
*/

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func testScriptDirClone(t *testing.T, tests []string, directory string) {
	if err := os.Chdir(directory); err != nil {
		t.Error(err)
	}
	defer os.Chdir("..")
	for _, script := range tests {
		fmt.Printf("testing %s/%s\n", directory, script)
		testScriptCompile(t, script)
		L := NewState(Options{
			RegistrySize:        1024 * 20,
			CallStackSize:       1024,
			IncludeGoStackTrace: true,
		})
		L.SetMx(maxMemory)
		snapshot := SaveSnapshot(L)
		C := LoadSnapshot(snapshot)

		fmt.Printf("L %v \n : C: %v\n", L, C)
		if err := C.DoFile(script); err != nil {
			t.Error(err)
		}
		C.Close()
	}
}

// func TestClonedLStateIsClosed(t *testing.T) {
// 	L := NewState()
// 	L.Close()
// 	C := L.Clone().(*LState)
// 	errorIfNotEqual(t, true, C.IsClosed())
// }

func TestClonedCallStackOverflowWhenFixed(t *testing.T) {
	L := NewState(Options{
		CallStackSize: 3,
	})
	defer L.Close()

	C := L.Clone().(*LState)
	defer C.Close()

	// expect fixed stack implementation by default (for backwards compatibility)
	stack := C.stack
	if _, ok := stack.(*fixedCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, C, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      print(_printregs())
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TesClonetGetAndReplace(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Replace(1, LString("b"))
	L.Replace(0, LString("c"))
	C1 := L.Clone().(*LState)
	defer C1.Close()

	errorIfNotEqual(t, LNil, C1.Get(0))
	errorIfNotEqual(t, LNil, C1.Get(-10))
	errorIfNotEqual(t, C1.Env, C1.Get(EnvironIndex))
	errorIfNotEqual(t, LString("b"), C1.Get(1))

	L.Push(LString("c"))
	L.Push(LString("d"))
	L.Replace(-2, LString("e"))
	C2 := L.Clone().(*LState)
	defer C2.Close()

	errorIfNotEqual(t, LString("e"), C2.Get(-2))

	registry := L.NewTable()
	L.Replace(RegistryIndex, registry)
	L.G.Registry = registry
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(RegistryIndex, LNil)
		return 0
	}, "registry must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		env := L.NewTable()
		L.Replace(EnvironIndex, env)
		errorIfNotEqual(t, env, L.Get(EnvironIndex))
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(EnvironIndex, LNil)
		return 0
	}, "environment must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		gbl := L.NewTable()
		L.Replace(GlobalsIndex, gbl)
		errorIfNotEqual(t, gbl, L.G.Global)
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(GlobalsIndex, LNil)
		return 0
	}, "_G must be a table")

	L2 := NewState()
	defer L2.Close()
	clo := L2.NewClosure(func(L2 *LState) int {
		L2.Replace(UpvalueIndex(1), LNumber(3))
		errorIfNotEqual(t, LNumber(3), L2.Get(UpvalueIndex(1)))
		return 0
	}, LNumber(1), LNumber(2))
	L2.SetGlobal("clo", clo)
	errorIfScriptFail(t, L2, `clo()`)
}

// func TestCloneRemove(t *testing.T) {
// 	L := NewState()
// 	defer L.Close()
// 	L.Push(LString("a"))
// 	L.Push(LString("b"))
// 	L.Push(LString("c"))

// 	L.Remove(4)
// 	C1 := L.Clone().(*LState)
// 	defer C1.Close()

// 	errorIfNotEqual(t, LString("a"), C1.Get(1))
// 	errorIfNotEqual(t, LString("b"), C1.Get(2))
// 	errorIfNotEqual(t, LString("c"), C1.Get(3))
// 	errorIfNotEqual(t, 3, C1.GetTop())

// 	L.Remove(3)
// 	C2 := L.Clone().(*LState)
// 	defer C2.Close()

// 	errorIfNotEqual(t, LString("a"), C2.Get(1))
// 	errorIfNotEqual(t, LString("b"), C2.Get(2))
// 	errorIfNotEqual(t, LNil, C2.Get(3))
// 	errorIfNotEqual(t, 2, C2.GetTop())
// 	L.Push(LString("c"))

// 	L.Remove(-10)

// 	C3 := L.Clone().(*LState)
// 	defer C3.Close()

// 	errorIfNotEqual(t, LString("a"), C3.Get(1))
// 	errorIfNotEqual(t, LString("b"), C3.Get(2))
// 	errorIfNotEqual(t, LString("c"), C3.Get(3))
// 	errorIfNotEqual(t, 3, C3.GetTop())

// 	L.Remove(2)

// 	C4 := L.Clone().(*LState)
// 	defer C4.Close()

// 	errorIfNotEqual(t, LString("a"), C4.Get(1))
// 	errorIfNotEqual(t, LString("c"), C4.Get(2))
// 	errorIfNotEqual(t, LNil, C4.Get(3))
// 	errorIfNotEqual(t, 2, C4.GetTop())
// }

func TestCloneToInt(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfNotEqual(t, 10, C.ToInt(1))
	errorIfNotEqual(t, 99, C.ToInt(2))
	errorIfNotEqual(t, 0, C.ToInt(3))
}

func TestCloneToInt64(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfNotEqual(t, int64(10), C.ToInt64(1))
	errorIfNotEqual(t, int64(99), C.ToInt64(2))
	errorIfNotEqual(t, int64(0), C.ToInt64(3))
}

func TestCloneoNumber(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfNotEqual(t, LNumber(10), C.ToNumber(1))
	errorIfNotEqual(t, LNumber(99.9), C.ToNumber(2))
	errorIfNotEqual(t, LNumber(0), C.ToNumber(3))
}

func TestCloneToString(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfNotEqual(t, "10", C.ToString(1))
	errorIfNotEqual(t, "99.9", C.ToString(2))
	errorIfNotEqual(t, "", C.ToString(3))
}

func TestCloneToTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfFalse(t, C.ToTable(1) == nil, "index 1 must be nil")
	errorIfFalse(t, C.ToTable(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, C.Get(3), C.ToTable(3))
}

func TestCloneToFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewFunction(func(L *LState) int { return 0 }))
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfFalse(t, C.ToFunction(1) == nil, "index 1 must be nil")
	errorIfFalse(t, C.ToFunction(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, C.Get(3), C.ToFunction(3))
}

func TestCloneToUserData(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	L.Push(L.NewUserData())
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfFalse(t, C.ToUserData(1) == nil, "index 1 must be nil")
	errorIfFalse(t, C.ToUserData(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, C.Get(3), C.ToUserData(3))
}

func TestCloneToChannel(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LNumber(10))
	L.Push(LString("99.9"))
	var ch chan LValue
	L.Push(LChannel(ch))
	C := L.Clone().(*LState)
	defer C.Close()

	errorIfFalse(t, C.ToChannel(1) == nil, "index 1 must be nil")
	errorIfFalse(t, C.ToChannel(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, ch, C.ToChannel(3))
}

func TestClonePCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Register("f1", func(L *LState) int {
		panic("panic!")
		return 0
	})
	C1 := L.Clone().(*LState)
	defer C1.Close()

	// Normal
	errorIfScriptNotFail(t, L, `f1()`, "panic!")
	L.Push(L.GetGlobal("f1"))
	err := L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.Push(LString("by handler"))
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "by handler"), "")

	// Clone
	errorIfScriptNotFail(t, C1, `f1()`, "panic!")
	C1.Push(C1.GetGlobal("f1"))
	cloneErr := C1.PCall(0, 0, C1.NewFunction(func(L *LState) int {
		C1.Push(LString("by handler"))
		return 1
	}))
	errorIfFalse(t, strings.Contains(cloneErr.Error(), "by handler"), "")

	C2 := L.Clone().(*LState)
	defer C2.Close()

	// Normal
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "error!"), "")

	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		panic("panicc!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "panicc!"), "")

	// Clone
	cloneErr = C2.PCall(0, 0, C2.NewFunction(func(L *LState) int {
		C2.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(cloneErr.Error(), "error!"), "")

	cloneErr = C2.PCall(0, 0, C2.NewFunction(func(L *LState) int {
		panic("panicc!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(cloneErr.Error(), "panicc!"), "")

}
func TestGluaClone(t *testing.T) {
	testScriptDirClone(t, gluaTests, "_glua-tests")
}

func TestLuaClone(t *testing.T) {
	testScriptDirClone(t, luaTests, "_lua5.1-tests")
}
