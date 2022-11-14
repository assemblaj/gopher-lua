package lua

import (
	"context"
	"fmt"
	"os"
)

type LValueType int

const (
	LTNil LValueType = iota
	LTBool
	LTNumber
	LTString
	LTFunction
	LTUserData
	LTThread
	LTTable
	LTChannel
)

var lValueNames = [9]string{"nil", "boolean", "number", "string", "function", "userdata", "thread", "table", "channel"}

func (vt LValueType) String() string {
	return lValueNames[int(vt)]
}

type LValue interface {
	String() string
	Type() LValueType
	// to reduce `runtime.assertI2T2` costs, this method should be used instead of the type assertion in heavy paths(typically inside the VM).
	assertFloat64() (float64, bool)
	// to reduce `runtime.assertI2T2` costs, this method should be used instead of the type assertion in heavy paths(typically inside the VM).
	assertString() (string, bool)
	// to reduce `runtime.assertI2T2` costs, this method should be used instead of the type assertion in heavy paths(typically inside the VM).
	assertFunction() (*LFunction, bool)
	Clone() LValue
}

// LVIsFalse returns true if a given LValue is a nil or false otherwise false.
func LVIsFalse(v LValue) bool { return v == LNil || v == LFalse }

// LVIsFalse returns false if a given LValue is a nil or false otherwise true.
func LVAsBool(v LValue) bool { return v != LNil && v != LFalse }

// LVAsString returns string representation of a given LValue
// if the LValue is a string or number, otherwise an empty string.
func LVAsString(v LValue) string {
	switch sn := v.(type) {
	case LString, LNumber:
		return sn.String()
	default:
		return ""
	}
}

// LVCanConvToString returns true if a given LValue is a string or number
// otherwise false.
func LVCanConvToString(v LValue) bool {
	switch v.(type) {
	case LString, LNumber:
		return true
	default:
		return false
	}
}

// LVAsNumber tries to convert a given LValue to a number.
func LVAsNumber(v LValue) LNumber {
	switch lv := v.(type) {
	case LNumber:
		return lv
	case LString:
		if num, err := parseNumber(string(lv)); err == nil {
			return num
		}
	}
	return LNumber(0)
}

type LNilType struct{}

func (nl *LNilType) String() string                     { return "nil" }
func (nl *LNilType) Type() LValueType                   { return LTNil }
func (nl *LNilType) assertFloat64() (float64, bool)     { return 0, false }
func (nl *LNilType) assertString() (string, bool)       { return "", false }
func (nl *LNilType) assertFunction() (*LFunction, bool) { return nil, false }
func (nl *LNilType) Clone() LValue                      { return LNil }

var LNil = LValue(&LNilType{})

type LBool bool

func (bl LBool) String() string {
	if bool(bl) {
		return "true"
	}
	return "false"
}
func (bl LBool) Type() LValueType                   { return LTBool }
func (bl LBool) assertFloat64() (float64, bool)     { return 0, false }
func (bl LBool) assertString() (string, bool)       { return "", false }
func (bl LBool) assertFunction() (*LFunction, bool) { return nil, false }
func (bl LBool) Clone() LValue {
	if bool(bl) {
		return LTrue
	}
	return LFalse
}

var LTrue = LBool(true)
var LFalse = LBool(false)

type LString string

func (st LString) String() string                     { return string(st) }
func (st LString) Type() LValueType                   { return LTString }
func (st LString) assertFloat64() (float64, bool)     { return 0, false }
func (st LString) assertString() (string, bool)       { return string(st), true }
func (st LString) assertFunction() (*LFunction, bool) { return nil, false }
func (st LString) Clone() LValue                      { return st }

// fmt.Formatter interface
func (st LString) Format(f fmt.State, c rune) {
	switch c {
	case 'd', 'i':
		if nm, err := parseNumber(string(st)); err != nil {
			defaultFormat(nm, f, 'd')
		} else {
			defaultFormat(string(st), f, 's')
		}
	default:
		defaultFormat(string(st), f, c)
	}
}

func (nm LNumber) String() string {
	if isInteger(nm) {
		return fmt.Sprint(int64(nm))
	}
	return fmt.Sprint(float64(nm))
}

func (nm LNumber) Type() LValueType                   { return LTNumber }
func (nm LNumber) assertFloat64() (float64, bool)     { return float64(nm), true }
func (nm LNumber) assertString() (string, bool)       { return "", false }
func (nm LNumber) assertFunction() (*LFunction, bool) { return nil, false }
func (nm LNumber) Clone() LValue                      { return nm }

// fmt.Formatter interface
func (nm LNumber) Format(f fmt.State, c rune) {
	switch c {
	case 'q', 's':
		defaultFormat(nm.String(), f, c)
	case 'b', 'c', 'd', 'o', 'x', 'X', 'U':
		defaultFormat(int64(nm), f, c)
	case 'e', 'E', 'f', 'F', 'g', 'G':
		defaultFormat(float64(nm), f, c)
	case 'i':
		defaultFormat(int64(nm), f, 'd')
	default:
		if isInteger(nm) {
			defaultFormat(int64(nm), f, c)
		} else {
			defaultFormat(float64(nm), f, c)
		}
	}
}

type LTable struct {
	Metatable LValue

	array   []LValue
	dict    map[LValue]LValue
	strdict map[string]LValue
	keys    []LValue
	k2i     map[LValue]int
}

func (tb *LTable) String() string                     { return fmt.Sprintf("table: %p", tb) }
func (tb *LTable) Type() LValueType                   { return LTTable }
func (tb *LTable) assertFloat64() (float64, bool)     { return 0, false }
func (tb *LTable) assertString() (string, bool)       { return "", false }
func (tb *LTable) assertFunction() (*LFunction, bool) { return nil, false }
func (tb *LTable) Clone() LValue {
	result := &LTable{}
	*result = *tb

	result.Metatable = tb.Metatable.Clone()

	result.array = make([]LValue, len(tb.array))
	copy(result.array, tb.array)
	// for i, v := range tb.array {
	// 	result.array[i] = v.Clone()
	// }

	result.dict = make(map[LValue]LValue)
	for k, v := range tb.dict {
		result.dict[k] = v
	}
	// for k, v := range tb.dict {
	// 	result.dict[k] = v.Clone()
	// }

	result.strdict = make(map[string]LValue)
	for k, v := range tb.strdict {
		result.strdict[k] = v
	}
	// for k, v := range tb.strdict {
	// 	result.strdict[k] = v.Clone()
	// }

	result.keys = make([]LValue, len(tb.keys))
	copy(result.keys, tb.keys)
	// for i, v := range tb.keys {
	// 	result.keys[i] = v.Clone()
	// }

	return result
}

type LFunction struct {
	IsG       bool
	Env       *LTable
	Proto     *FunctionProto
	GFunction LGFunction
	Upvalues  []*Upvalue
}
type LGFunction func(*LState) int

func (fn *LFunction) String() string                     { return fmt.Sprintf("function: %p", fn) }
func (fn *LFunction) Type() LValueType                   { return LTFunction }
func (fn *LFunction) assertFloat64() (float64, bool)     { return 0, false }
func (fn *LFunction) assertString() (string, bool)       { return "", false }
func (fn *LFunction) assertFunction() (*LFunction, bool) { return fn, true }
func (fn *LFunction) Clone() LValue {
	result := &LFunction{}
	*result = *fn

	// if fn.Env != nil {
	// 	result.Env = fn.Env.Clone().(*LTable)
	// }

	// if fn.Proto != nil {
	// 	result.Proto = fn.Proto.Clone()
	// }

	result.Upvalues = make([]*Upvalue, len(fn.Upvalues))
	copy(result.Upvalues, fn.Upvalues)
	// for i, v := range fn.Upvalues {
	// 	if v != nil {
	// 		result.Upvalues[i] = v.Clone()
	// 	}
	// }

	return result
}

type Global struct {
	MainThread    *LState
	CurrentThread *LState
	Registry      *LTable
	Global        *LTable

	builtinMts map[int]LValue
	tempFiles  []*os.File
	gccount    int32
}

func (g *Global) Clone() (result *Global) {
	result = &Global{}
	*result = *g

	if g.Registry != nil {
		*result.Registry = *g.Registry.Clone().(*LTable)
	}

	if g.Global != nil {
		*result.Global = *g.Global.Clone().(*LTable)
	}

	result.builtinMts = make(map[int]LValue)
	for k, v := range g.builtinMts {
		if v != nil {
			result.builtinMts[k] = v.Clone()
		}
	}

	result.tempFiles = make([]*os.File, len(g.tempFiles))
	copy(result.tempFiles, g.tempFiles)
	return
}

type LState struct {
	G       *Global
	Parent  *LState
	Env     *LTable
	Panic   func(*LState)
	Dead    bool
	Options Options

	stop         int32
	reg          *registry
	stack        callFrameStack
	alloc        *allocator
	currentFrame *callFrame
	wrapped      bool
	uvcache      *Upvalue
	hasErrorFunc bool
	mainLoop     func(*LState, *callFrame)
	ctx          context.Context
}

func (ls *LState) String() string                     { return fmt.Sprintf("thread: %p", ls) }
func (ls *LState) Type() LValueType                   { return LTThread }
func (ls *LState) assertFloat64() (float64, bool)     { return 0, false }
func (ls *LState) assertString() (string, bool)       { return "", false }
func (ls *LState) assertFunction() (*LFunction, bool) { return nil, false }
func (l *LState) Clone() LValue {
	result := &LState{}
	*result = *l

	if l.G != nil {
		*result.G = *l.G.Clone()
	}

	// // result.Parent = l.Parent

	if l.Env != nil {
		*result.Env = *l.Env.Clone().(*LTable)
		result.Env = result.G.Global
	}

	if l.reg != nil {
		*result.reg = *l.reg.clone()
		result.reg.handler = result
	}

	if l.stack != nil {
		result.stack = l.stack.Clone()
	}

	if l.currentFrame != nil {
		*result.currentFrame = *l.currentFrame.clone()
	}

	if l.uvcache != nil {
		*result.uvcache = *l.uvcache.Clone()
	}

	return result
}

type LUserData struct {
	Value     interface{}
	Env       *LTable
	Metatable LValue
}

func (ud *LUserData) String() string                     { return fmt.Sprintf("userdata: %p", ud) }
func (ud *LUserData) Type() LValueType                   { return LTUserData }
func (ud *LUserData) assertFloat64() (float64, bool)     { return 0, false }
func (ud *LUserData) assertString() (string, bool)       { return "", false }
func (ud *LUserData) assertFunction() (*LFunction, bool) { return nil, false }
func (ud *LUserData) Clone() LValue {
	result := &LUserData{}
	*result = *ud

	if ud.Env != nil {
		result.Env = ud.Env.Clone().(*LTable)
	}

	if ud.Metatable != nil {
		result.Metatable = ud.Metatable.Clone()
	}
	return result
}

type LChannel chan LValue

func (ch LChannel) String() string                     { return fmt.Sprintf("channel: %p", ch) }
func (ch LChannel) Type() LValueType                   { return LTChannel }
func (ch LChannel) assertFloat64() (float64, bool)     { return 0, false }
func (ch LChannel) assertString() (string, bool)       { return "", false }
func (ch LChannel) assertFunction() (*LFunction, bool) { return nil, false }
func (ch LChannel) Clone() LValue                      { return ch }
