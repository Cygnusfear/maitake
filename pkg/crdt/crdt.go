// Package crdt provides a character-level CRDT text type backed by y-crdt (yrs)
// running as WASM via wazero. No cgo, no Rust toolchain needed at build time.
package crdt

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func randomClientID() uint64 {
	var buf [8]byte
	rand.Read(buf[:])
	return binary.LittleEndian.Uint64(buf[:])
}

//go:embed yrs.wasm
var yrsWasm []byte

// TextDoc is a CRDT text document backed by a YDoc.
// All operations are thread-safe (serialized through the WASM instance).
type TextDoc struct {
	mu      sync.Mutex
	runtime wazero.Runtime
	mod     wazero.CompiledModule
	inst    api.Module
	ctx     context.Context
}

// New creates a new empty CRDT text document.
func New() (*TextDoc, error) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiled, err := r.CompileModule(ctx, yrsWasm)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("compiling yrs wasm: %w", err)
	}

	inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("yrs"))
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("instantiating yrs wasm: %w", err)
	}

	doc := &TextDoc{
		runtime: r,
		mod:     compiled,
		inst:    inst,
		ctx:     ctx,
	}

	// Initialize empty doc
	docNew := inst.ExportedFunction("doc_new")
	if docNew == nil {
		r.Close(ctx)
		return nil, fmt.Errorf("doc_new not exported")
	}
	results, err := docNew.Call(ctx)
	if err != nil || results[0] != 0 {
		r.Close(ctx)
		return nil, fmt.Errorf("doc_new failed")
	}

	return doc, nil
}

// Load creates a TextDoc from a serialized state (from Save).
// Each Load generates a unique client ID to ensure correct CRDT merge.
func Load(state []byte) (*TextDoc, error) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	compiled, err := r.CompileModule(ctx, yrsWasm)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("compiling yrs wasm: %w", err)
	}

	inst, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("yrs"))
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("instantiating yrs wasm: %w", err)
	}

	doc := &TextDoc{
		runtime: r,
		mod:     compiled,
		inst:    inst,
		ctx:     ctx,
	}

	// Load state with a unique client ID for this peer
	clientID := randomClientID()
	ptr, err := doc.writeToWasm(state)
	if err != nil {
		doc.Close()
		return nil, err
	}

	docLoadFn := inst.ExportedFunction("doc_load_with_client")
	if docLoadFn != nil {
		results, err := docLoadFn.Call(ctx, clientID, uint64(ptr), uint64(len(state)))
		doc.freeWasm(ptr, uint32(len(state)))
		if err != nil || int32(results[0]) != 0 {
			doc.Close()
			return nil, fmt.Errorf("doc_load_with_client failed")
		}
	} else {
		docLoadFn = inst.ExportedFunction("doc_load")
		results, err := docLoadFn.Call(ctx, uint64(ptr), uint64(len(state)))
		doc.freeWasm(ptr, uint32(len(state)))
		if err != nil || int32(results[0]) != 0 {
			doc.Close()
			return nil, fmt.Errorf("doc_load failed")
		}
	}

	return doc, nil
}

// Insert inserts text at the given character position.
func (d *TextDoc) Insert(index uint32, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data := []byte(text)
	ptr, err := d.writeToWasm(data)
	if err != nil {
		return err
	}
	defer d.freeWasm(ptr, uint32(len(data)))

	fn := d.inst.ExportedFunction("text_insert")
	results, err := fn.Call(d.ctx, uint64(index), uint64(ptr), uint64(len(data)))
	if err != nil || results[0] != 0 {
		return fmt.Errorf("text_insert failed")
	}
	return nil
}

// Delete removes len characters starting at index.
func (d *TextDoc) Delete(index, length uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	fn := d.inst.ExportedFunction("text_delete")
	results, err := fn.Call(d.ctx, uint64(index), uint64(length))
	if err != nil || results[0] != 0 {
		return fmt.Errorf("text_delete failed")
	}
	return nil
}

// Content returns the full text content.
func (d *TextDoc) Content() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	fn := d.inst.ExportedFunction("text_content")
	results, err := fn.Call(d.ctx)
	if err != nil || results[0] != 0 {
		return "", fmt.Errorf("text_content failed")
	}
	data, err := d.readResult()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Length returns the text length in characters.
func (d *TextDoc) Length() (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	fn := d.inst.ExportedFunction("text_length")
	results, err := fn.Call(d.ctx)
	if err != nil {
		return 0, err
	}
	return int(int32(results[0])), nil
}

// Save encodes the full document state as bytes.
func (d *TextDoc) Save() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	fn := d.inst.ExportedFunction("doc_save")
	results, err := fn.Call(d.ctx)
	if err != nil || results[0] != 0 {
		return nil, fmt.Errorf("doc_save failed")
	}
	return d.readResult()
}

// StateVector returns the current state vector (for computing diffs).
func (d *TextDoc) StateVector() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	fn := d.inst.ExportedFunction("doc_state_vector")
	results, err := fn.Call(d.ctx)
	if err != nil || results[0] != 0 {
		return nil, fmt.Errorf("doc_state_vector failed")
	}
	return d.readResult()
}

// Diff computes an update containing changes since the given state vector.
func (d *TextDoc) Diff(stateVector []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	ptr, err := d.writeToWasm(stateVector)
	if err != nil {
		return nil, err
	}
	defer d.freeWasm(ptr, uint32(len(stateVector)))

	fn := d.inst.ExportedFunction("doc_diff")
	results, err := fn.Call(d.ctx, uint64(ptr), uint64(len(stateVector)))
	if err != nil || results[0] != 0 {
		return nil, fmt.Errorf("doc_diff failed")
	}
	return d.readResult()
}

// Apply merges a remote update into this document.
func (d *TextDoc) Apply(update []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	ptr, err := d.writeToWasm(update)
	if err != nil {
		return err
	}
	defer d.freeWasm(ptr, uint32(len(update)))

	fn := d.inst.ExportedFunction("doc_apply")
	results, err := fn.Call(d.ctx, uint64(ptr), uint64(len(update)))
	if err != nil || results[0] != 0 {
		return fmt.Errorf("doc_apply failed")
	}
	return nil
}

// Close releases all resources.
func (d *TextDoc) Close() {
	d.runtime.Close(d.ctx)
}

// ── Internal helpers ─────────────────────────────────────────────────────

func (d *TextDoc) writeToWasm(data []byte) (uint32, error) {
	allocFn := d.inst.ExportedFunction("alloc")
	results, err := allocFn.Call(d.ctx, uint64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("alloc failed: %w", err)
	}
	ptr := uint32(results[0])
	if !d.inst.Memory().Write(ptr, data) {
		return 0, fmt.Errorf("memory write failed")
	}
	return ptr, nil
}

func (d *TextDoc) freeWasm(ptr, len uint32) {
	deallocFn := d.inst.ExportedFunction("dealloc")
	deallocFn.Call(d.ctx, uint64(ptr), uint64(len))
}

func (d *TextDoc) readResult() ([]byte, error) {
	ptrFn := d.inst.ExportedFunction("get_result_ptr")
	lenFn := d.inst.ExportedFunction("get_result_len")

	ptrResults, err := ptrFn.Call(d.ctx)
	if err != nil {
		return nil, err
	}
	lenResults, err := lenFn.Call(d.ctx)
	if err != nil {
		return nil, err
	}

	ptr := uint32(ptrResults[0])
	size := uint32(lenResults[0])

	data, ok := d.inst.Memory().Read(ptr, size)
	if !ok {
		return nil, fmt.Errorf("memory read failed")
	}

	// Copy so it's safe after WASM memory changes
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}
