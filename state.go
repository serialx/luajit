package luajit

/*
#cgo LDFLAGS: -lluajit
#cgo linux LDFLAGS: -lm -ldl

#include <lua.h>
#include <stddef.h>
#include <stdlib.h>

extern lua_State*	newstate(void);
extern int			load(lua_State*, void*, size_t, const char*);
*/
import "C"
import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"unsafe"
)

type Debug struct {
	d *C.lua_Debug
}

// A Gofunction is a Go function that may be registered with the Lua
// interpreter and called by Lua programs.
//
// In order to communicate properly with Lua, a Go function must use the
// following protocol, which defines the way parameters and results are
// passed: a Go function receives its arguments from Lua in its stack
// in direct order (the first argument is pushed first). So, when the
// function starts, s.Gettop returns the number of arguments received by the
// function. The first argument (if any) is at index 1 and its last argument
// is at index s.Gettop. To return values to Lua, a Go function just pushes
// them onto the stack, in direct order (the first result is pushed first),
// and returns the number of results. Any other value in the stack below
// the results will be properly discarded by Lua. Like a Lua function,
// a Go function called by Lua can also return many results.
//
// As an example, the following function receives a variable number of
// numerical arguments and returns their average and sum:
//
// 	func foo(s *luajit.State) int {
// 		n := s.Gettop()		// number of arguments
// 		sum := 0.0
// 		for i := 1; i <= n; i++ {
// 			if !s.Isnumber(i) {
// 				s.Pushstring("incorrect argument")
// 				s.Error()
// 			}
// 			sum += s.Tonumber(i)
// 		}
// 		s.Pushnumber(sum/n)	// first result
// 		s.Pushnumber(sum)	// second result
// 		return 2		// number of results
// 	}
type Gofunction func(*State) int

// A State keeps all state of a Lua interpreter.
type State struct {
	l *C.lua_State
}

// Creates a new State, giving Lua a basic allocator, and returns a
// pointer to it.
func Newstate() *State {
	return &State{C.newstate()}
}

// TODO
func (s *State) Atpanic(panicf Gofunction) Gofunction {
	return panicf
}

// Calls a function.
//
// To call a function you must use the following protocol: first,
// the function to be called is pushed onto the stack; then, the
// arguments to the function are pushed in direct order; that is, the
// first argument is pushed first. Finally you call Call; nargs is the
// number of arguments that you pushed onto the stack. All arguments
// and the function value are popped from the stack when the function
// is called. The function results are pushed onto the stack when the
// function returns. The number of results is adjusted to nresults, unless
// nresults is luajit.Multret. In this case, all results from the function
// are pushed. Lua takes care that the returned values fit into the stack
// space. The function results are pushed onto the stack in direct order
// (the first result is pushed first), so that after the call the last
// result is on the top of the stack.
//
// Any error inside the called function is propagated upwards (with
// a longjmp).
func (s *State) Call(nargs, nresults int) {
	C.lua_call(s.l, C.int(nargs), C.int(nresults))
}

// Ensures that there are at least extra free stack slots in the stack. It
// returns false if it cannot grow the stack to that size. This function
// never shrinks the stack; if the stack is already larger than the new
// size, it is left unchanged.
func (s *State) Checkstack(extra int) bool {
	return C.lua_checkstack(s.l, C.int(extra)) == 1
}

// Destroys all objects in the given Lua state (calling the corresponding
// garbage-collection metamethods, if any) and frees all dynamic memory
// used by this state. On several platforms, you may not need to call
// this function, because all resources are naturally released when the
// host program ends. On the other hand, long-running programs, such as
// a daemon or a web server, might need to release states as soon as they
// are not needed, to avoid growing too large.
func (s *State) Close() {
	C.lua_close(s.l)
}

// Concatenates the n values at the top of the stack, pops them, and
// leaves the result at the top. If n is 1, the result is the single
// value on the stack (that is, the function does nothing); if n is 0,
// the result is the empty string. Concatenation is performed following
// the usual semantics of Lua.
func (s *State) Concat(n int) {
	C.lua_concat(s.l, C.int(n))
}

//export goreadchunk
func goreadchunk(reader, buf unsafe.Pointer, buflen C.size_t) int {
	r := (*bytes.Reader)(reader)
	cb := (*C.char)(buf)
	leng := int(buflen)
	var b []byte
	hdr := (*reflect.SliceHeader)((unsafe.Pointer(&b)))
	hdr.Cap = leng
	hdr.Len = leng
	hdr.Data = uintptr(unsafe.Pointer(cb))

	n, err := r.Read(b)
	if err != nil {
		return 0
	}
	return n
}

// Reads a Lua chunk from an *io.Reader. If there are no errors, Load pushes
// the compiled chunk as a Lua function on top of the stack, and returns nil.
//
// Chunk reading is buffered; the bufsize argument chooses the size
// of the internal buffer, which must be a number greater than 0.
//
// The chunkname argument gives a name to the chunk, which is used for
// error messages and in debug information
//
// Load only loads a chunk; it does not run it.
//
// Load automatically detects whether the chunk is text or binary, and
// loads it accordingly (see program luac).
//
func (s *State) Load(chunk *io.Reader, bufsize int, chunkname string) error {
	cs := C.CString(chunkname)
	defer C.free(unsafe.Pointer(cs))
	r := int(C.load(s.l, unsafe.Pointer(chunk), C.size_t(bufsize), (*C.char)(unsafe.Pointer(cs))))
	return err2msg(r)
}

// Creates a new empty table and pushes it onto the stack. The new table
// has space pre-allocated for narr array elements and nrec non-array
// elements. This pre-allocation is useful when you know exactly how many
// elements the table will have. Otherwise you can use the function Newtable.
func (s *State) Createtable(narr, nrec int) {
	C.lua_createtable(s.l, C.int(narr), C.int(nrec))
}

// Creates a new empty table and pushes it onto the stack. It is equivalent
// to Createable(0, 0).
func (s *State) Newtable() {
	s.Createtable(0, 0)
}

// Pops a key from the stack, and pushes a key-value pair from the table
// at the given index (the "next" pair after the given key). If there are
// no more elements in the table, then Next returns 0 (and pushes nothing).
//
// A typical traversal looks like this:
// 	// table is in the stack at index 't'
// 	s.Pushnil()	// first key
// 	for s.Next(t) != 0 {
// 		// use key (at index -2) and value (index -1)
// 		fmt.Printf("%s - %s\n",
// 			s.Typename(s.Type(-2)),
// 			s.Typename(s.Type(-1)))
// 		// remove value, keep key for next iteration
// 		s.Pop(1)
// 	}
func (s *State) Next(index int) int {
	return int(C.lua_next(s.l, C.int(index)))
}

func (s *State) Getinfo(what string, ar *Debug) int {
	cs := C.CString(what)
	defer C.free(unsafe.Pointer(cs))
	return int(C.lua_getinfo(s.l, cs, ar.d))
}

// Pushes onto the stack the value t[k], where t is the value at the
// given valid index.
func (s *State) Getfield(index int, k string) {
	cs := C.CString(k)
	defer C.free(unsafe.Pointer(cs))
	C.lua_getfield(s.l, C.int(index), cs)
}

// Pushes onto the stack the value of the global name.
func (s *State) Getglobal(name string) {
	s.Getfield(Globalsindex, name)
}

// Pushes onto the stack the value t[k], where t is the value at the
// given valid index and k is the value at the top of the stack.
//
// This function pops the key from the stack (putting the resulting value
// in its place). As in Lua, this function may trigger a metamethod for
// the "index" event
func (s *State) Gettable(index int) {
	C.lua_gettable(s.l, C.int(index))
}

// Returns the index of the top element in the stack. Because indices start
// at 1, this result is equal to the number of elements in the stack (and
// so 0 means an empty stack).
func (s *State) Gettop() int {
	return int(C.lua_gettop(s.l))
}

// Returns the "length" of the value at the given valid index: for
// strings, this is the string length; for tables, this is the result of
// the length operator ('#'); for userdata, this is the size of the block
// of memory allocated for the userdata; for other values, it is 0.
func (s *State) Objlen(index int) int {
	return int(C.lua_objlen(s.l, C.int(index)))
}

// Accepts any valid index, or 0, and sets the stack top to this
// index. If the new top is larger than the old one, then the new elements
// are filled with nil. If index is 0, then all stack elements are removed.
func (s *State) Settop(index int) {
	C.lua_settop(s.l, C.int(index))
}

func (s *State) Pop(index int) {
	s.Settop(-index - 1)
}

// Moves the top element into the given valid index, shifting up the elements
// above this index to open space. Cannot be called with a pseudo-index,
// because a pseudo-index is not an actual stack position.
func (s *State) Insert(index int) {
	C.lua_insert(s.l, C.int(index))
}

// Pops a value from the stack and sets it as the new value of global name.
func (s *State) Setglobal(name string) {
	s.Setfield(Globalsindex, name)
}

// Does the equivalent to t[k] = v, where t is the value at the given valid
// index and v is the value at the top of the stack.
//
// This function pops the value from the stack. As in Lua, this function
// may trigger a metamethod for the "newindex" event
func (s *State) Setfield(index int, k string) {
	ck := C.CString(k)
	defer C.free(unsafe.Pointer(ck))
	C.lua_setfield(s.l, C.int(index), ck)
}

// Returns true if the value at the given valid index is a function
// (either Go or Lua), and false otherwise.
func (s *State) Isfunction(index int) bool {
	return s.Type(index) == Tfunction
}

// Returns true if the value at the given valid index is a table,
// and false otherwise.
func (s *State) Istable(index int) bool {
	return s.Type(index) == Ttable
}

// Returns true if the value at the given valid index is light
// userdata, and false otherwise.
func (s *State) Islightuserdata(index int) bool {
	return s.Type(index) == Tlightuserdata
}

// Returns true if the value at the given valid index is a Go function,
// and false otherwise.
func (s *State) Isgofunction(index int) bool {
	return int(C.lua_iscfunction(s.l, C.int(index))) == 1
}

// Returns true if the value at the given valid index is nil,
// and false otherwise.
func (s *State) Isnil(index int) bool {
	return s.Type(index) == Tnil
}

// Returns true if the value at the given valid index has type
// boolean, and false otherwise.
func (s *State) Isboolean(index int) bool {
	return s.Type(index) == Tboolean
}

// Returns true if the value at the given valid index is a thread,
// and false otherwise.
func (s *State) Isthread(index int) bool {
	return s.Type(index) == Tthread
}

// Returns true if the given valid index is not valid (that is, it
// refers to an element outside the current stack), and false otherwise.
func (s *State) Isnone(index int) bool {
	return s.Type(index) == Tnone
}

// Returns true if the given valid index is not valid (that is, it
// refers to an element outside the current stack) or if the value at this
// index is nil, and false otherwise.
func (s *State) Isnoneornil(index int) bool {
	return s.Type(index) <= 0
}

// Pushes a boolean value with value b onto the stack.
func (s *State) Pushboolean(b bool) {
	if b {
		C.lua_pushboolean(s.l, 1)
	} else {
		C.lua_pushboolean(s.l, 0)
	}
}

// void lua_pushcclosure (lua_State *L, lua_CFunction fn, int n);
//
// Pushes a new C closure onto the stack.
//
// When a C function is created, it is possible to associate some values
// with it, thus creating a C closure (see §3.4); these values are then
// accessible to the function whenever it is called. To associate values
// with a C function, first these values should be pushed onto the stack
// (when there are multiple values, the first value is pushed first). Then
// lua_pushcclosure is called to create and push the C function onto the
// stack, with the argument n telling how many values should be associated
// with the function. lua_pushcclosure also pops these values from the stack.
//
// The maximum value for n is 255.

// void lua_pushcfunction (lua_State *L, lua_CFunction f);
//
// Pushes a C function onto the stack. This function receives a pointer to
// a C function and pushes onto the stack a Lua value of type function that,
// when called, invokes the corresponding C function.
//
// Any function to be registered in Lua must follow the correct protocol
// to receive its parameters and return its results (see lua_CFunction).
//
// lua_pushcfunction is defined as a macro:
//
// #define lua_pushcfunction(L,f)  lua_pushcclosure(L,f,0)

// Formats a string and pushes it into the stack.  Provides all formatting
// verbs found in package fmt.  Returns a pointer to the resultant
// formatted string.
func (s *State) Pushfstring(format string, v ...interface{}) *string {
	str := fmt.Sprintf(format, v)
	cs := C.CString(str)
	defer C.free(unsafe.Pointer(cs))
	C.lua_pushstring(s.l, cs)
	return &str
}

// Pushes a number with value n onto the stack.
func (s *State) Pushinteger(n int) {
	C.lua_pushinteger(s.l, C.lua_Integer(n))
}

// void lua_pushlightuserdata (lua_State *L, void *p);
//
// Pushes a light userdata onto the stack.
//
// Userdata represent C values in Lua. A light userdata represents a
// pointer. It is a value (like a number): you do not create it, it has no
// individual metatable, and it is not collected (as it was never created). A
// light userdata is equal to "any" light userdata with the same C address.

// Pushes a nil value onto the stack.
func (s *State) Pushnil() {
	C.lua_pushnil(s.l)
}

// Pushes a number with value n onto the stack.
func (s *State) Pushnumber(n float64) {
	C.lua_pushnumber(s.l, C.lua_Number(n))
}

// Pushes the string str onto the stack.
func (s *State) Pushstring(str string) {
	cs := C.CString(str)
	defer C.free(unsafe.Pointer(cs))
	C.lua_pushstring(s.l, cs)
}

// Pushes the thread represented by s onto the stack. Returns 1 if this
// thread is the main thread of its state.
// FIXME?
func (s *State) Pushthread() int {
	return int(C.lua_pushthread(s.l))
}

// Pushes a copy of the element at the given valid index onto the stack.
func (s *State) Pushvalue(index int) {
	C.lua_pushvalue(s.l, C.int(index))
}

// Returns true if the two values at valid indices i1 and i2 are
// primitively equal (that is, without calling metamethods). Otherwise
// returns false. Also returns false if any of the indices are invalid.
func (s *State) Rawequal(i1, i2 int) bool {
	return int(C.lua_rawequal(s.l, C.int(i1), C.int(i2))) == 1
}

// Similar to Gettable, but does a raw access (i.e., without metamethods).
func (s *State) Rawget(index int) {
	C.lua_rawget(s.l, C.int(index))
}

// Pushes onto the stack the value t[n], where t is the value at the given
// valid index. The access is raw; that is, it does not invoke metamethods.
func (s *State) Rawgeti(index, n int) {
	C.lua_rawgeti(s.l, C.int(index), C.int(n))
}

// Similar to Settable, but does a raw assignment (i.e., without
// metamethods).
func (s *State) Rawset(index int) {
	C.lua_rawset(s.l, C.int(index))
}

// Does the equivalent of t[n] = v, where t is the value at the given valid
// index and v is the value at the top of the stack.
//
// This function pops the value from the stack. The assignment is raw;
// that is, it does not invoke metamethods.
func (s *State) Rawseti(index, n int) {
	C.lua_rawseti(s.l, C.int(index), C.int(n))
}

// Removes the element at the given valid index, shifting down the elements
// above this index to fill the gap. Cannot be called with a pseudo-index,
// because a pseudo-index is not an actual stack position.
func (s *State) Remove(index int) {
	C.lua_remove(s.l, C.int(index))
}

// Moves the top element into the given position (and pops it), without
// shifting any element (therefore replacing the value at the given
// position).
func (s *State) Replace(index int) {
	C.lua_replace(s.l, C.int(index))
}

// Starts and resumes a coroutine in a given thread.  To start a
// coroutine, you first create a new thread (see Newthread); then you
// push onto its stack the main function plus any arguments; then you
// call Resume, with narg being the number of arguments. This call returns
// when the coroutine suspends or finishes its execution. When it returns,
// the stack contains all values passed to Yield, or all values returned
// by the body function. Resume returns luajit.Yield if the coroutine
// yields, 0 if the coroutine finishes its execution without errors,
// or an error code in case of errors (see Pcall). In case of errors,
// the stack is not unwound, so you can use the debug API over it. The
// error message is on the top of the stack. To restart a coroutine, you
// put on its stack only the values to be passed as results from yield,
// and then call Resume.
func (s *State) Resume(narg int) int {
	return int(C.lua_resume(s.l, C.int(narg)))
}

// Returns the status of the thread s.
//
// The status can be 0 for a normal thread, an error code if the thread
// finished its execution with an error, or LUA_YIELD if the thread is
// suspended.
func (s *State) Status() int {
	return int(C.lua_status(s.l))
}

func (s *State) Strlen(index int) int {
	return s.Objlen(index)
}

// Converts the Lua value at the given valid index to a Go boolean
// value. Like all tests in Lua, Toboolean returns true for any Lua value
// different from false and nil; otherwise it returns false. It also returns
// false when called with a non-valid index. (If you want to accept only
// actual boolean values, use Isboolean to test the value's type.)
func (s *State) Toboolean(index int) bool {
	return int(C.lua_toboolean(s.l, C.int(index))) == 1
}

// Converts a value at the given valid index to a Go function. That
// value must be a Go function; otherwise, returns nil.
//
// TODO
func (s *State) Togofunction(index int) Gofunction {
	return func(st *State) int {
		return 0
	}
}

// Converts the Lua value at the given valid index to a Go int. The Lua
// value must be a number or a string convertible to a number; otherwise,
// Tointeger returns 0.
//
// If the number is not an integer, it is truncated in some non-specified
// way.
func (s *State) Tointeger(index int) int {
	return int(C.lua_tointeger(s.l, C.int(index)))
}

// Converts the Lua value at the given valid index to a float64. The
// Lua value must be a number or a string convertible to a number; otherwise,
// Tonumber returns 0.
func (s *State) Tonumber(index int) float64 {
	return float64(C.lua_tonumber(s.l, C.int(index)))
}

// TODO?
// const void *lua_topointer (lua_State *L, int index);

// Converts the Lua value at the given valid index to a Go
// string. The Lua value must be a string or a number; otherwise,
// the function returns an empty string. If the value is a number, then
// Tostring also changes the actual value in the stack to a string. (This
// change confuses Next when Tostring is applied to keys during a table
// traversal).  The string always has a zero ('\0') after its last
// character (as in C), but can contain other zeros in its body.
func (s *State) Tostring(index int) string {
	str := C.lua_tolstring(s.l, C.int(index), nil)
	if str == nil {
		return ""
	}
	return C.GoString(str)
}

// Converts the value at the given valid index to a Lua thread
// (represented as a *State). This value must be a thread; otherwise,
// the function returns nil.
func (s *State) Tothread(index int) *State {
	t := C.lua_tothread(s.l, C.int(index))
	if t == nil {
		return nil
	}
	return &State{t}
}

// TODO?
// void *lua_touserdata (lua_State *L, int index);
//
// If the value at the given valid index is a full userdata, returns
// its block address. If the value is a light userdata, returns its
// pointer. Otherwise, returns NULL.

// Returns the type of the value in the given valid index, or Tnone for
// a non-valid index (that is, an index to an "empty" stack position). The
// types returned by lua_type are coded by the following constants defined in
// const.go: Tnil, Tnumber, Tboolean, Tstring, Ttable, Tfunction, Tuserdata,
// Tthread, and Tlightuserdata.
func (s *State) Type(index int) int {
	return int(C.lua_type(s.l, C.int(index)))
}

// Returns the name of the type encoded by the value tp, which must be one
// the values returned by Type.
func (s *State) Typename(index int) string {
	return C.GoString(C.lua_typename(s.l, C.int(index)))
}

// Exchange values between different threads of the /same/ global state.
//
// This function pops n values from the stack from, and pushes them onto
// the stack to.
func (to *State) Xmove(from *State, n int) {
	C.lua_xmove(from.l, to.l, C.int(n))
}

// Yields a coroutine.
//
// This function should only be called as the return expression of a Go
// function, as follows:
// 	return s.Yield(nresults)
//
// When a Go function calls Yield in that way, the running coroutine
// suspends its execution, and the call to Resume that started this coroutine
// returns. The parameter nresults is the number of values from the stack
// that are passed as results to Resume.
func (s *State) Yield(nresults int) int {
	return int(C.lua_yield(s.l, C.int(nresults)))
}
