//! Times `conforms` and `diagnose` on ordinary input: a list of n closed records
//! `{name text, age opt int, tags list-of text}`, with age's type an int ("plain") or a
//! three-way union of leaves ("union"). It measures the checker, not seeded conformance,
//! and no check runs it; `scripts/bench.py` runs it beside the other cores (ADR 0013).
//!
//! ```text
//! cargo run --release --example bench -- 200000 plain
//! ```

use std::alloc::{GlobalAlloc, Layout, System};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Instant;

use horos::conforms;
use horos::diagnose::diagnose;
use ontos_core::Value;
use ontos_data::{encode_int, encode_list, encode_map, encode_null, encode_text};

/// Counts the bytes allocated, as Go's `TotalAlloc` does.
struct Counting;

static ALLOCATED: AtomicU64 = AtomicU64::new(0);

unsafe impl GlobalAlloc for Counting {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        ALLOCATED.fetch_add(layout.size() as u64, Ordering::Relaxed);
        unsafe { System.alloc(layout) }
    }
    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        unsafe { System.dealloc(ptr, layout) }
    }
    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        ALLOCATED.fetch_add(new_size as u64, Ordering::Relaxed);
        unsafe { System.realloc(ptr, layout, new_size) }
    }
}

#[global_allocator]
static GLOBAL: Counting = Counting;

fn a(s: &str) -> Value {
    Value::atom(s.as_bytes().to_vec())
}

fn d(ctor: &str, mut args: Vec<Value>) -> Value {
    let mut items = vec![a(&format!("horos/{ctor}"))];
    items.append(&mut args);
    Value::tuple(items)
}

fn field(name: &str, presence: &str, ty: Value) -> Value {
    d("field", vec![encode_text(name), a(presence), ty])
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let [n, mode] = args.as_slice() else {
        eprintln!("usage: bench <n> plain|union");
        std::process::exit(2);
    };
    let n: usize = n.parse().expect("n must be a non-negative integer");
    let age = if mode == "union" {
        d(
            "union",
            vec![
                d("exact", vec![encode_null()]),
                d("scalar", vec![a("int")]),
                d("scalar", vec![a("utf8-text")]),
            ],
        )
    } else {
        d("scalar", vec![a("int")])
    };
    let desc = d(
        "list-of",
        vec![d(
            "record",
            vec![
                a("closed"),
                field("name", "req", d("scalar", vec![a("utf8-text")])),
                field("age", "opt", age),
                field(
                    "tags",
                    "req",
                    d("list-of", vec![d("scalar", vec![a("utf8-text")])]),
                ),
            ],
        )],
    );
    let items: Vec<Value> = (0..n)
        .map(|i| {
            encode_map(&[
                (encode_text("age"), encode_int(i as i128)),
                (encode_text("name"), encode_text(&format!("n{i}"))),
                (
                    encode_text("tags"),
                    encode_list(&[encode_text("a"), encode_text("b"), encode_text("c")]),
                ),
            ])
            .expect("a canonical map")
        })
        .collect();
    let v = encode_list(&items);

    let measure = |run: &dyn Fn() -> String| {
        let before = ALLOCATED.load(Ordering::Relaxed);
        let start = Instant::now();
        let out = run();
        let ms = start.elapsed().as_millis();
        let mb = (ALLOCATED.load(Ordering::Relaxed) - before) as f64 / 1e6;
        (out, ms, mb)
    };
    let (ok, cms, cmb) = measure(&|| conforms(&v, &desc).to_string());
    let (dg, dms, dmb) = measure(&|| format!("{:?}", diagnose(&v, &desc)).to_lowercase());
    println!(
        "{{\"core\":\"rust\",\"n\":{n},\"mode\":\"{mode}\",\"conforms\":{ok},\"conforms_ms\":{cms},\"conforms_mb\":{cmb:.1},\"diagnose\":\"{dg}\",\"diagnose_ms\":{dms},\"diagnose_mb\":{dmb:.1}}}"
    );
}
