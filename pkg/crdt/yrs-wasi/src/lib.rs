// Thin WASI wrapper around yrs for maitake doc CRDT.
// Exports minimal text operations callable from Go via wazero.
//
// Memory protocol:
// - Caller allocates memory via alloc(), writes input, calls function
// - Function writes output via set_result(), caller reads via get_result_ptr/len
// - Caller frees via dealloc()
//
// YDoc state is held in a global (single-threaded WASI).

use std::sync::Mutex;
use yrs::{Doc, GetString, ReadTxn, Text, TextRef, Transact, StateVector, Update};
use yrs::updates::decoder::Decode;
use yrs::updates::encoder::Encode;

static DOC: Mutex<Option<Doc>> = Mutex::new(None);
static RESULT_BUF: Mutex<Vec<u8>> = Mutex::new(Vec::new());

// ── Memory management ────────────────────────────────────────────────────

#[no_mangle]
pub extern "C" fn alloc(len: u32) -> *mut u8 {
    let mut buf = Vec::with_capacity(len as usize);
    let ptr = buf.as_mut_ptr();
    std::mem::forget(buf);
    ptr
}

#[no_mangle]
pub extern "C" fn dealloc(ptr: *mut u8, len: u32) {
    unsafe {
        drop(Vec::from_raw_parts(ptr, len as usize, len as usize));
    }
}

fn set_result(data: &[u8]) {
    let mut buf = RESULT_BUF.lock().unwrap();
    buf.clear();
    buf.extend_from_slice(data);
}

#[no_mangle]
pub extern "C" fn get_result_ptr() -> *const u8 {
    RESULT_BUF.lock().unwrap().as_ptr()
}

#[no_mangle]
pub extern "C" fn get_result_len() -> u32 {
    RESULT_BUF.lock().unwrap().len() as u32
}

// ── Doc lifecycle ────────────────────────────────────────────────────────

/// Create a new empty YDoc with a given client ID. Returns 0 on success.
#[no_mangle]
pub extern "C" fn doc_new() -> i32 {
    let mut doc = DOC.lock().unwrap();
    *doc = Some(Doc::new());
    0
}

/// Create a new empty YDoc with a specific client ID. Returns 0 on success.
#[no_mangle]
pub extern "C" fn doc_new_with_client(client_id: u64) -> i32 {
    let mut doc = DOC.lock().unwrap();
    *doc = Some(Doc::with_client_id(client_id));
    0
}

/// Load a YDoc from a state update (binary) with a given client ID.
/// The client_id ensures different peers have distinct identities for merge.
/// Returns 0 on success, -1 on error.
#[no_mangle]
pub extern "C" fn doc_load_with_client(client_id: u64, ptr: *const u8, len: u32) -> i32 {
    let data = unsafe { std::slice::from_raw_parts(ptr, len as usize) };
    let new_doc = Doc::with_client_id(client_id);
    let update = match Update::decode_v1(data) {
        Ok(u) => u,
        Err(_) => return -1,
    };
    {
        let mut txn = new_doc.transact_mut();
        if txn.apply_update(update).is_err() {
            return -1;
        }
    }
    let mut doc = DOC.lock().unwrap();
    *doc = Some(new_doc);
    0
}

/// Load a YDoc from a state update (binary). Returns 0 on success, -1 on error.
/// WARNING: Uses Doc::new() which may generate non-unique client IDs in WASM.
/// Prefer doc_load_with_client() for multi-peer scenarios.
#[no_mangle]
pub extern "C" fn doc_load(ptr: *const u8, len: u32) -> i32 {
    let data = unsafe { std::slice::from_raw_parts(ptr, len as usize) };
    let new_doc = Doc::new();
    let update = match Update::decode_v1(data) {
        Ok(u) => u,
        Err(_) => return -1,
    };
    {
        let mut txn = new_doc.transact_mut();
        if txn.apply_update(update).is_err() {
            return -1;
        }
    }
    let mut doc = DOC.lock().unwrap();
    *doc = Some(new_doc);
    0
}

/// Encode the full YDoc state as a binary update. Result available via get_result_ptr/len.
#[no_mangle]
pub extern "C" fn doc_save() -> i32 {
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let txn = doc.transact();
    let update = txn.encode_state_as_update_v1(&StateVector::default());
    set_result(&update);
    0
}

/// Encode a diff since a given state vector. Input: state vector bytes.
/// Result: update bytes via get_result_ptr/len.
#[no_mangle]
pub extern "C" fn doc_diff(sv_ptr: *const u8, sv_len: u32) -> i32 {
    let sv_data = unsafe { std::slice::from_raw_parts(sv_ptr, sv_len as usize) };
    let sv = match StateVector::decode_v1(sv_data) {
        Ok(sv) => sv,
        Err(_) => return -1,
    };
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let txn = doc.transact();
    let update = txn.encode_state_as_update_v1(&sv);
    set_result(&update);
    0
}

/// Apply a binary update to the current doc. Returns 0 on success.
#[no_mangle]
pub extern "C" fn doc_apply(ptr: *const u8, len: u32) -> i32 {
    let data = unsafe { std::slice::from_raw_parts(ptr, len as usize) };
    let update = match Update::decode_v1(data) {
        Ok(u) => u,
        Err(_) => return -1,
    };
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let mut txn = doc.transact_mut();
    match txn.apply_update(update) {
        Ok(_) => 0,
        Err(_) => -1,
    }
}

/// Get the state vector of the current doc. Result via get_result_ptr/len.
#[no_mangle]
pub extern "C" fn doc_state_vector() -> i32 {
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let txn = doc.transact();
    let sv = txn.state_vector().encode_v1();
    set_result(&sv);
    0
}

// ── Text operations ──────────────────────────────────────────────────────

fn get_text(doc: &Doc) -> TextRef {
    doc.get_or_insert_text("content")
}

/// Get the full text content. Result via get_result_ptr/len.
#[no_mangle]
pub extern "C" fn text_content() -> i32 {
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let text = get_text(doc);
    let txn = doc.transact();
    let content = text.get_string(&txn);
    set_result(content.as_bytes());
    0
}

/// Insert text at a position. Input: the text bytes at ptr/len, position as index.
#[no_mangle]
pub extern "C" fn text_insert(index: u32, ptr: *const u8, len: u32) -> i32 {
    let data = unsafe { std::slice::from_raw_parts(ptr, len as usize) };
    let text_str = match std::str::from_utf8(data) {
        Ok(s) => s,
        Err(_) => return -1,
    };
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let text = get_text(doc);
    let mut txn = doc.transact_mut();
    text.insert(&mut txn, index, text_str);
    0
}

/// Delete len characters starting at index.
#[no_mangle]
pub extern "C" fn text_delete(index: u32, len: u32) -> i32 {
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let text = get_text(doc);
    let mut txn = doc.transact_mut();
    text.remove_range(&mut txn, index, len);
    0
}

/// Get the text length (in UTF-8 chars).
#[no_mangle]
pub extern "C" fn text_length() -> i32 {
    let doc = DOC.lock().unwrap();
    let doc = match doc.as_ref() {
        Some(d) => d,
        None => return -1,
    };
    let text = get_text(doc);
    let txn = doc.transact();
    text.len(&txn) as i32
}
