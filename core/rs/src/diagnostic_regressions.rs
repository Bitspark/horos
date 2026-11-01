use crate::{conforms, diagnose::{diagnose, diagnose_wf, Diagnosis}, wf, MAX_RECURSION_DEPTH};
use ontos_core::Value;
use ontos_data::encode_text;

fn a(s: &str) -> Value { Value::atom(s.as_bytes()) }
fn d(ctor: &str, args: Vec<Value>) -> Value {
    let mut items = vec![a(&format!("horos/{ctor}"))];
    items.extend(args);
    Value::tuple(items)
}
fn seqs(n: usize, mut ty: Value) -> Value { for _ in 0..n { ty = d("seq", vec![ty]); } ty }
fn field(ty: Value) -> Value { d("field", vec![encode_text("next"), a("opt"), ty]) }

#[test]
fn binary_identity() {
    let x = Value::atom([0x80]);
    let y = Value::atom([0x81]);
    let label = d("compound", vec![x.clone()]);
    let v = Value::tuple([y.clone()]);
    assert!(wf(&label));
    assert!(!conforms(&v, &label));
    assert_eq!(diagnose(&v, &label), Diagnosis::Mismatch);
    let ty = d("rec", vec![x.clone(), d("union", vec![d("exact", vec![a("end")]), d("tuple", vec![d("rec", vec![y, d("tuple", vec![d("ref", vec![x])])])])])]);
    let v = Value::tuple([Value::tuple([a("end")])]);
    assert!(wf(&ty) && conforms(&v, &ty));
    assert_eq!(diagnose(&v, &ty), Diagnosis::Conforms);
}

#[test]
fn wf_depth_and_scope() {
    std::thread::Builder::new().stack_size(64 * 1024 * 1024).spawn(|| {
        for inner in [d("rec", vec![a("x"), seqs(2, d("any", vec![]))]), d("record", vec![a("open"), field(seqs(2, d("any", vec![])))])] {
            let at = seqs(MAX_RECURSION_DEPTH - 3, inner.clone());
            let over = seqs(MAX_RECURSION_DEPTH - 2, inner);
            assert!(wf(&at));
            assert_eq!(diagnose_wf(&at), Diagnosis::Conforms);
            assert!(!wf(&over));
            assert!(matches!(diagnose_wf(&over), Diagnosis::Exhausted(_)));
            assert!(matches!(diagnose(&Value::tuple([]), &over), Diagnosis::Exhausted(_)));
        }
        let deep = d("rec", vec![a("x"), seqs(MAX_RECURSION_DEPTH + 1, d("any", vec![]))]);
        assert!(matches!(diagnose_wf(&deep), Diagnosis::Exhausted(_)));
        let guarded = d("rec", vec![a("x"), d("record", vec![a("open"), field(d("ref", vec![a("x")]))])]);
        assert!(wf(&guarded));
        assert_eq!(diagnose_wf(&guarded), Diagnosis::Conforms);
        for bad in [d("ref", vec![a("x")]), d("rec", vec![a("x"), d("ref", vec![a("x")])])] {
            assert_eq!(diagnose_wf(&bad), Diagnosis::NotWellFormed);
        }
    }).unwrap().join().unwrap();
}
