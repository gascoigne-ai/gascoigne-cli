// Package ortrt runs an ONNX graph through the ONNX Runtime C API without cgo:
// the shared library is opened at run time (purego) and its API table is read
// directly, so the helper cross-compiles with CGO_ENABLED=0. The slots below
// are those of ORT_API_VERSION 29 (offsetof in onnxruntime_c_api.h).
package ortrt

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

// apiVersion is the C API revision of the pinned ONNX Runtime release.
const apiVersion = 29

// Slots of OrtApi, in pointer-sized units.
const (
	slotGetErrorMessage   = 2
	slotCreateEnv         = 3
	slotCreateSession     = 7
	slotRun               = 9
	slotCreateSessOptions = 10
	slotCreateTensor      = 49
	slotGetTensorData     = 51
	slotCreateCPUMemInfo  = 69
	slotReleaseEnv        = 92
	slotReleaseStatus     = 93
	slotReleaseMemInfo    = 94
	slotReleaseSession    = 95
	slotReleaseValue      = 96
	slotReleaseSessOpts   = 100
	slotAppendCUDAV2      = 204
	slotCreateCUDAOptions = 205
	slotReleaseCUDAOpts   = 208
	slotAppendProvider    = 216
)

const (
	logWarning        = 2
	arenaAllocator    = 1
	memTypeDefault    = 0
	elementTypeInt64  = 7
	sizeofPointerBits = 3 // log2 of the pointer size the slot arithmetic assumes
)

// Runtime is a loaded ONNX Runtime library with its environment.
type Runtime struct {
	env uintptr
	api api
}

// api holds the C functions in use. Handles are uintptr (C pointers), Go
// memory is unsafe.Pointer; every function returns an OrtStatus*, 0 on success,
// except the release functions and GetErrorMessage.
type api struct {
	getErrorMessage   func(status uintptr) uintptr
	createEnv         func(level int32, logID unsafe.Pointer, out *uintptr) uintptr
	createSession     func(env uintptr, path unsafe.Pointer, opts uintptr, out *uintptr) uintptr
	run               func(sess, runOpts uintptr, inNames, ins unsafe.Pointer, nIn uintptr, outNames unsafe.Pointer, nOut uintptr, outs unsafe.Pointer) uintptr
	createSessOptions func(out *uintptr) uintptr
	createTensor      func(info uintptr, data unsafe.Pointer, bytes uintptr, shape unsafe.Pointer, rank uintptr, typ int32, out *uintptr) uintptr
	getTensorData     func(value uintptr, out *uintptr) uintptr
	createCPUMemInfo  func(alloc, mem int32, out *uintptr) uintptr
	releaseEnv        func(uintptr)
	releaseStatus     func(uintptr)
	releaseMemInfo    func(uintptr)
	releaseSession    func(uintptr)
	releaseValue      func(uintptr)
	releaseSessOpts   func(uintptr)
	appendCUDAV2      func(opts, cuda uintptr) uintptr
	createCUDAOptions func(out *uintptr) uintptr
	releaseCUDAOpts   func(uintptr)
	appendProvider    func(opts uintptr, name, keys, values unsafe.Pointer, n uintptr) uintptr
}

// Open loads the ONNX Runtime library at path (or its bare name, found on the
// library path) and creates the environment.
func Open(path string) (*Runtime, error) {
	h, err := openLibrary(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	sym, err := lookup(h, "OrtGetApiBase")
	if err != nil {
		return nil, err
	}
	var getBase func() uintptr
	purego.RegisterFunc(&getBase, sym)
	// OrtApiBase.GetApi is its first member.
	var getAPI func(version uint32) uintptr
	purego.RegisterFunc(&getAPI, *(*uintptr)(ptr(getBase())))
	table := getAPI(apiVersion)
	if table == 0 {
		return nil, fmt.Errorf("%s does not provide ONNX Runtime API %d (needs release 1.29)", path, apiVersion)
	}
	r := &Runtime{}
	r.bind(table)
	logID := cstr("prose")
	if err := r.check(r.api.createEnv(logWarning, unsafe.Pointer(&logID[0]), &r.env)); err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}
	return r, nil
}

func (r *Runtime) bind(table uintptr) {
	reg := func(fn any, slot int) {
		purego.RegisterFunc(fn, *(*uintptr)(ptr(table + uintptr(slot)<<sizeofPointerBits)))
	}
	a := &r.api
	reg(&a.getErrorMessage, slotGetErrorMessage)
	reg(&a.createEnv, slotCreateEnv)
	reg(&a.createSession, slotCreateSession)
	reg(&a.run, slotRun)
	reg(&a.createSessOptions, slotCreateSessOptions)
	reg(&a.createTensor, slotCreateTensor)
	reg(&a.getTensorData, slotGetTensorData)
	reg(&a.createCPUMemInfo, slotCreateCPUMemInfo)
	reg(&a.releaseEnv, slotReleaseEnv)
	reg(&a.releaseStatus, slotReleaseStatus)
	reg(&a.releaseMemInfo, slotReleaseMemInfo)
	reg(&a.releaseSession, slotReleaseSession)
	reg(&a.releaseValue, slotReleaseValue)
	reg(&a.releaseSessOpts, slotReleaseSessOpts)
	reg(&a.appendCUDAV2, slotAppendCUDAV2)
	reg(&a.createCUDAOptions, slotCreateCUDAOptions)
	reg(&a.releaseCUDAOpts, slotReleaseCUDAOpts)
	reg(&a.appendProvider, slotAppendProvider)
}

// Close releases the environment.
func (r *Runtime) Close() { r.api.releaseEnv(r.env) }

// Output names the graph output to read and how many floats it holds per
// token: final_scores has one, logits two (drop, keep).
type Output struct {
	Name  string
	Width int
}

// Session is a loaded graph taking input_ids and attention_mask and
// returning the Output.
type Session struct {
	rt      *Runtime
	handle  uintptr
	memInfo uintptr
	out     Output
}

// NewSession loads the graph on device: "cuda", "coreml" or "cpu". A provider
// the library lacks is an error, so callers can fall back.
func (r *Runtime) NewSession(model, device string, out Output) (*Session, error) {
	var opts uintptr
	if err := r.check(r.api.createSessOptions(&opts)); err != nil {
		return nil, err
	}
	defer r.api.releaseSessOpts(opts)
	if err := r.appendProvider(opts, device); err != nil {
		return nil, err
	}
	s := &Session{rt: r, out: out}
	path := modelPath(model)
	if err := r.check(r.api.createSession(r.env, path, opts, &s.handle)); err != nil {
		return nil, err
	}
	runtime.KeepAlive(path)
	if err := r.check(r.api.createCPUMemInfo(arenaAllocator, memTypeDefault, &s.memInfo)); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (r *Runtime) appendProvider(opts uintptr, device string) error {
	switch device {
	case "cpu":
		return nil
	case "cuda":
		var cuda uintptr
		if err := r.check(r.api.createCUDAOptions(&cuda)); err != nil {
			return err
		}
		defer r.api.releaseCUDAOpts(cuda)
		return r.check(r.api.appendCUDAV2(opts, cuda))
	case "coreml":
		name := cstr("CoreML")
		return r.check(r.api.appendProvider(opts, unsafe.Pointer(&name[0]), nil, nil, 0))
	}
	return fmt.Errorf("unknown device %q", device)
}

// Scores runs one sequence and returns Width floats per token.
func (s *Session) Scores(ids []int64) ([]float32, error) {
	r, n := s.rt, len(ids)
	mask := make([]int64, n)
	for i := range mask {
		mask[i] = 1
	}
	shape := [2]int64{1, int64(n)}
	var in [2]uintptr
	for i, data := range [2][]int64{ids, mask} {
		err := r.check(r.api.createTensor(s.memInfo, unsafe.Pointer(&data[0]), uintptr(n)*8,
			unsafe.Pointer(&shape[0]), 2, elementTypeInt64, &in[i]))
		if err != nil {
			s.release(in[:i])
			return nil, err
		}
	}
	defer s.release(in[:])
	names := [2][]byte{cstr("input_ids"), cstr("attention_mask")}
	inNames := [2]unsafe.Pointer{unsafe.Pointer(&names[0][0]), unsafe.Pointer(&names[1][0])}
	outName := cstr(s.out.Name)
	outNames := [1]unsafe.Pointer{unsafe.Pointer(&outName[0])}
	var out uintptr
	err := r.check(r.api.run(s.handle, 0, unsafe.Pointer(&inNames[0]), unsafe.Pointer(&in[0]), 2,
		unsafe.Pointer(&outNames[0]), 1, unsafe.Pointer(&out)))
	runtime.KeepAlive(ids)
	runtime.KeepAlive(mask)
	if err != nil {
		return nil, err
	}
	defer r.api.releaseValue(out)
	var data uintptr
	if err := r.check(r.api.getTensorData(out, &data)); err != nil {
		return nil, err
	}
	return append([]float32(nil), unsafe.Slice((*float32)(ptr(data)), n*s.out.Width)...), nil
}

func (s *Session) release(values []uintptr) {
	for _, v := range values {
		s.rt.api.releaseValue(v)
	}
}

// Close releases the session.
func (s *Session) Close() {
	if s.memInfo != 0 {
		s.rt.api.releaseMemInfo(s.memInfo)
	}
	if s.handle != 0 {
		s.rt.api.releaseSession(s.handle)
	}
}

// check turns an OrtStatus* into an error and frees it.
func (r *Runtime) check(status uintptr) error {
	if status == 0 {
		return nil
	}
	defer r.api.releaseStatus(status)
	return errors.New(goString(r.api.getErrorMessage(status)))
}

func cstr(s string) []byte { return append([]byte(s), 0) }

// ptr converts a C address to a pointer without go vet's uintptr warning:
// the memory is C's, never Go's.
func ptr(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func goString(addr uintptr) string {
	if addr == 0 {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(ptr(addr), n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(ptr(addr)), n))
}
