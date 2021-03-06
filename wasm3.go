package wasm3

/*
#cgo darwin CFLAGS: -Iinclude
#cgo darwin LDFLAGS: -L${SRCDIR}/lib/darwin -lm3
#cgo linux CFLAGS: -Iinclude
#cgo linux LDFLAGS: -L${SRCDIR}/lib/linux -lm3 -lm

#include "wasm3.h"
#include "m3_api_libc.h"
#include "m3_api_wasi.h"
#include "m3_env.h"
#include "go-wasm3.h"

#include <stdio.h>

typedef uint32_t __wasi_size_t;
#include "extra/wasi_core.h"

IM3Function module_get_function(IM3Module i_module, int index);
IM3Function module_get_imported_function(IM3Module i_module, int index);
int call(IM3Function i_function, uint32_t i_argc, int i_argv[]);
int get_allocated_memory_length(IM3Runtime i_runtime);
u8* get_allocated_memory(IM3Runtime i_runtime);
const void * mowrapper(IM3Runtime runtime, uint64_t * _sp, void * _mem);
int attachFunction(IM3Module i_module, char* moduleName, char* functionName, char* signature);
void* m3ApiOffsetToPtr(void* offset, void* _mem);
const char* function_get_import_module(IM3Function i_function);
const char* function_get_import_field(IM3Function i_function);
int findFunction(IM3Function * o_function, IM3Runtime i_runtime, const char * const i_moduleName, const char * const i_functionName);
u8 function_get_arg_type(IM3Function i_function, int index);

typedef struct wasi_iovec_t
{
    __wasi_size_t buf;
    __wasi_size_t buf_len;
} wasi_iovec_t;
*/
import "C"

import (
	"errors"
	"reflect"
	"sync"
	"unsafe"
)

// RuntimeT is an alias for IM3Runtime
type RuntimeT C.IM3Runtime

// EnvironmentT is an alias for IM3Environment
type EnvironmentT C.IM3Environment

// ModuleT is an alias for IM3Module
type ModuleT C.IM3Module

// FunctionT is an alias for IM3Function
type FunctionT C.IM3Function

// FuncTypeT is an alias for IM3FuncType
type FuncTypeT C.IM3FuncType

// ResultT is an alias for M3Result
type ResultT C.M3Result

// WasiIoVec is an alias for wasi_iovec_t
type WasiIoVec C.wasi_iovec_t

// CallbackFunction is the signature for callbacks
type CallbackFunction func(runtime RuntimeT, sp unsafe.Pointer, mem unsafe.Pointer) int

var slotsToCallbacks = make(map[int]CallbackFunction)

// GetBuf return the internal buffer index
func (w *WasiIoVec) GetBuf() uint32 {
	return uint32(w.buf)
}

// GetBufLen return the buffer len
func (w *WasiIoVec) GetBufLen() int {
	return int(w.buf_len)
}

//export dynamicFunctionWrapper
func dynamicFunctionWrapper(runtime RuntimeT, _sp unsafe.Pointer, _mem unsafe.Pointer, slot uint64) int {
	return slotsToCallbacks[int(slot)](runtime, _sp, _mem)
}

var (
	errParseModule      = errors.New("Parse error")
	errLoadModule       = errors.New("Load error")
	errFuncLookupFailed = errors.New("Function lookup failed")
)

// Config holds the runtime and environment configuration
type Config struct {
	Environment    *Environment
	StackSize      uint
	EnableWASI     bool
	EnableSpecTest bool
}

// Runtime wraps a WASM3 runtime
type Runtime struct {
	ptr RuntimeT
	cfg *Config
}

// Ptr returns a IM3Runtime pointer
func (r *Runtime) Ptr() C.IM3Runtime {
	return (C.IM3Runtime)(r.ptr)
}

// Load wraps the parse and load module calls.
// This will be replaced by env.ParseModule and Runtime.LoadModule.
func (r *Runtime) Load(wasmBytes []byte) (*Module, error) {
	result := C.m3Err_none
	bytes := C.CBytes(wasmBytes)
	length := len(wasmBytes)
	var module C.IM3Module
	result = C.m3_ParseModule(
		r.cfg.Environment.Ptr(),
		&module,
		(*C.uchar)(bytes),
		C.uint(length),
	)
	if result != nil {
		return nil, errParseModule
	}
	result = C.m3_LoadModule(
		r.Ptr(),
		module,
	)
	if result != nil {
		return nil, errLoadModule
	}
	result = C.m3_LinkSpecTest(r.Ptr().modules)
	if result != nil {
		return nil, errors.New("LinkSpecTest failed")
	}
	if r.cfg.EnableWASI {
		C.m3_LinkWASI(r.Ptr().modules)
	}
	m := NewModule((ModuleT)(module))
	return m, nil
}

var lock = sync.Mutex{}

// AttachFunction binds a callable function to a module+func
func (r *Runtime) AttachFunction(moduleName string, functionName string, signature string, callback CallbackFunction) {
	_moduleName := C.CString(moduleName)
	defer C.free(unsafe.Pointer(_moduleName))

	_functionName := C.CString(functionName)
	defer C.free(unsafe.Pointer(_functionName))

	_signature := C.CString(signature)
	defer C.free(unsafe.Pointer(_signature))

	lock.Lock()
	slot := C.attachFunction(r.Ptr().modules, _moduleName, _functionName, _signature)
	slotsToCallbacks[int(slot)] = callback
	lock.Unlock()
}

// LoadModule wraps m3_LoadModule and returns a module object
func (r *Runtime) LoadModule(module *Module) (*Module, error) {
	result := C.m3Err_none
	result = C.m3_LoadModule(
		r.Ptr(),
		module.Ptr(),
	)
	if result != nil {
		return nil, errLoadModule
	}
	if r.cfg.EnableSpecTest {
		C.m3_LinkSpecTest(r.Ptr().modules)
	}
	if r.cfg.EnableWASI {
		C.m3_LinkWASI(r.Ptr().modules)
	}
	return module, nil
}

// FindFunction calls m3_FindFunction and returns a call function
func (r *Runtime) FindFunction(funcName string) (FunctionWrapper, error) {
	result := C.m3Err_none
	var f C.IM3Function
	cFuncName := C.CString(funcName)
	defer C.free(unsafe.Pointer(cFuncName))
	result = C.m3_FindFunction(
		&f,
		r.Ptr(),
		cFuncName,
	)
	if result != nil {
		return nil, errFuncLookupFailed
	}
	fn := &Function{
		ptr: (FunctionT)(f),
	}
	// var fnWrapper FunctionWrapper
	// fnWrapper = fn.Call
	return FunctionWrapper(fn.Call), nil
}

// FindFunction does thins
func (r *Runtime) FindFunctionByModule(moduleName string, funcName string) (FunctionWrapper, error) {
	var f C.IM3Function

	cModuleName := C.CString(moduleName)
	defer C.free(unsafe.Pointer(cModuleName))

	cFuncName := C.CString(funcName)
	defer C.free(unsafe.Pointer(cFuncName))

	result := C.findFunction(
		&f,
		r.Ptr(),
		cModuleName,
		cFuncName,
	)

	if result != 0 {
		return nil, errFuncLookupFailed
	}

	fn := &Function{
		ptr: (FunctionT)(f),
	}
	// var fnWrapper FunctionWrapper
	// fnWrapper = fn.Call
	return FunctionWrapper(fn.Call), nil
}

// Destroy free calls m3_FreeRuntime
func (r *Runtime) Destroy() {
	C.m3_FreeRuntime(r.Ptr())
	r.cfg.Environment.Destroy()
}

// Memory allows access to runtime Memory.
// Taken from Wasmer extension: https://github.com/wasmerio/go-ext-wasm
func (r *Runtime) Memory() []byte {
	mem := C.get_allocated_memory(
		r.Ptr(),
	)
	var data = (*uint8)(mem)
	length := r.GetAllocatedMemoryLength()
	var header reflect.SliceHeader
	header = *(*reflect.SliceHeader)(unsafe.Pointer(&header))
	header.Data = uintptr(unsafe.Pointer(data))
	header.Len = int(length)
	header.Cap = int(length)
	return *(*[]byte)(unsafe.Pointer(&header))
}

// GetAllocatedMemoryLength returns the amount of allocated runtime memory
func (r *Runtime) GetAllocatedMemoryLength() int {
	length := C.get_allocated_memory_length(r.Ptr())
	return int(length)
}

// ParseModule is a helper that calls the same function in env.
func (r *Runtime) ParseModule(wasmBytes []byte) (*Module, error) {
	return r.cfg.Environment.ParseModule(wasmBytes)
}

func (r *Runtime) PrintRuntimeInfo() {
	C.m3_PrintRuntimeInfo(r.Ptr())

	C.m3_PrintM3Info()

	C.m3_PrintProfilerInfo()
}

// NewRuntime initializes a new runtime
// TODO: nativeStackInfo is passed as NULL
func NewRuntime(cfg *Config) *Runtime {
	// env *Environment, stackSize uint
	ptr := C.m3_NewRuntime(
		cfg.Environment.Ptr(),
		C.uint(cfg.StackSize),
		nil,
	)
	return &Runtime{
		ptr: (RuntimeT)(ptr),
		cfg: cfg,
	}
}

// Module wraps a WASM3 module.
type Module struct {
	ptr          ModuleT
	numFunctions int
	numImports   int
}

// Ptr returns a pointer to IM3Module
func (m *Module) Ptr() C.IM3Module {
	return (C.IM3Module)(m.ptr)
}

// GetFunction provides access to IM3Function->functions
func (m *Module) GetFunction(index uint) (*Function, error) {
	if uint(m.NumFunctions()) <= index {
		return nil, errFuncLookupFailed
	}
	ptr := C.module_get_function(m.Ptr(), C.int(index))
	name := C.GoString(ptr.name)
	return &Function{
		ptr:  (FunctionT)(ptr),
		Name: name,
	}, nil
}

func (f *Function) GetReturnType() uint8 {
	return uint8(f.ptr.funcType.returnType)
}

func (f *Function) GetNumArgs() uint32 {
	return uint32(f.ptr.funcType.numArgs)
}

func (f *Function) GetArgType(index int) uint8 {
	return uint8(C.function_get_arg_type(f.ptr, C.int(index)))
}

func (f *Function) GetSignature() string {
	// TODO this is completely wrong but should work for basic functions for the moment...
	s := "i("
	for i := uint32(0); i < f.GetNumArgs(); i++ {
		s += "i"
	}
	s += ")"
	return s
}

// GetFunctionByName is a helper to lookup functions by name
// TODO: could be optimized by caching function names and pointer on the Go side, right after the load call.
func (m *Module) GetFunctionByName(lookupName string) (*Function, error) {
	var fn *Function
	for i := 0; i < m.NumFunctions(); i++ {
		ptr := C.module_get_function(m.Ptr(), C.int(i))
		name := C.GoString(ptr.name)
		if name != lookupName {
			continue
		}
		fn = &Function{
			ptr:  (FunctionT)(ptr),
			Name: name,
		}
		return fn, nil
	}
	return nil, errFuncLookupFailed
}

// NumFunctions provides access to numFunctions.
func (m *Module) NumFunctions() int {
	// In case the number of functions hasn't been resolved yet, retrieve the int and keep it in the structure
	if m.numFunctions == -1 {
		m.numFunctions = int(m.Ptr().numFunctions)
	}
	return m.numFunctions
}

// NumImports provides access to numImports
func (m *Module) NumImports() int {
	if m.numImports == -1 {
		m.numImports = int(m.Ptr().numImports)
	}
	return m.numImports
}

// GetModule retreive the function's module
func (f *Function) GetModule() *Module {
	return NewModule(f.ptr.module)
}

func (f *Function) GetImportModule() *string {
	if f.ptr == nil {
		return nil
	}

	cs := C.function_get_import_module(f.ptr)
	if cs == nil {
		return nil
	}

	res := C.GoString(cs)
	return &res
}

func (f *Function) GetImportField() *string {
	if f.ptr == nil {
		return nil
	}

	cs := C.function_get_import_field(f.ptr)
	if cs == nil {
		return nil
	}

	res := C.GoString(cs)
	return &res
}

// Name gets the module's name
func (m *Module) Name() string {
	return C.GoString(m.ptr.name)
}

// NewModule wraps a WASM3 moduke
func NewModule(ptr ModuleT) *Module {
	return &Module{
		ptr:          ptr,
		numFunctions: -1,
		numImports:   -1,
	}
}

// Function is a function wrapper
type Function struct {
	ptr FunctionT
	// fnWrapper FunctionWrapper
	Name string
}

// FunctionWrapper is used to wrap WASM3 call methods and make the calls more idiomatic
// TODO: this is very limited, we need to handle input and output types appropriately
type FunctionWrapper func(args ...interface{}) (int, error)

// Ptr returns a pointer to IM3Function
func (f *Function) Ptr() C.IM3Function {
	return (C.IM3Function)(f.ptr)
}

// CallWithArgs wraps m3_CallWithArgs
func (f *Function) CallWithArgs(args ...string) {
	length := len(args)
	cArgs := make([]*C.char, length)
	for i, v := range args {
		cVal := C.CString(v)
		cArgs[i] = cVal
	}
	C.m3_CallWithArgs(f.Ptr(), C.uint(length), &cArgs[0])
}

// Call implements a better call function
// TODO: support diferent types
func (f *Function) Call(args ...interface{}) (int, error) {
	length := len(args)
	if length == 0 {
		result := C.call(f.Ptr(), 0, nil)
		if result == -1 {
			return int(result), errors.New(LastErrorString())
		}
		return int(result), nil
	}
	cArgs := make([]C.int, length)
	for i, v := range args {
		val := v.(int)
		n := C.int(val)
		cArgs[i] = n
	}
	result := C.call(f.Ptr(), C.uint(length), &cArgs[0])
	if result == -1 {
		return int(result), errors.New(LastErrorString())
	}
	return int(result), nil
}

func (f *Function) Call2(args []int) (int, error) {
	length := len(args)
	if length == 0 {
		result := C.call(f.Ptr(), 0, nil)
		if result == -1 {
			return int(result), errors.New(LastErrorString())
		}
		return int(result), nil
	}
	cArgs := make([]C.int, length)
	for i, v := range args {
		n := C.int(v)
		cArgs[i] = n
	}
	result := C.call(f.Ptr(), C.uint(length), &cArgs[0])
	if result == -1 {
		return int(result), errors.New(LastErrorString())
	}
	return int(result), nil
}

// Environment wraps a WASM3 environment
type Environment struct {
	ptr EnvironmentT
}

// ParseModule wraps m3_ParseModule
func (e *Environment) ParseModule(wasmBytes []byte) (*Module, error) {
	result := C.m3Err_none
	bytes := C.CBytes(wasmBytes)
	length := len(wasmBytes)
	var module C.IM3Module
	result = C.m3_ParseModule(
		e.Ptr(),
		&module,
		(*C.uchar)(bytes),
		C.uint(length),
	)
	if result != nil {
		return nil, errParseModule
	}
	return NewModule((ModuleT)(module)), nil
}

// Ptr returns a pointer to IM3Environment
func (e *Environment) Ptr() C.IM3Environment {
	return (C.IM3Environment)(e.ptr)
}

// Destroy calls m3_FreeEnvironment
func (e *Environment) Destroy() {
	C.m3_FreeEnvironment(e.Ptr())
}

// NewEnvironment initializes a new environment
func NewEnvironment() *Environment {
	ptr := C.m3_NewEnvironment()
	return &Environment{
		ptr: (EnvironmentT)(ptr),
	}
}
