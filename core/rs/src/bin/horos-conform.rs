//! Evaluate wf/conforms over ontos-codec-v1 hex.
use horos::{conforms, wf};
use ontos_core::Value;

fn decode(hex: &str) -> Result<Value, String> {
    if !hex.len().is_multiple_of(2) || !hex.bytes().all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b)) {
        return Err("invalid lowercase hex".into());
    }
    let bytes: Result<Vec<_>, _> = (0..hex.len()).step_by(2).map(|i| u8::from_str_radix(&hex[i..i+2], 16)).collect();
    ontos_codec::decode(&bytes.map_err(|e| e.to_string())?).map_err(|e| format!("{e:?}"))
}

fn run(args: &[String]) -> Result<bool, String> {
    match args {
        [op, descriptor] if op == "wf" => Ok(wf(&decode(descriptor)?)),
        [op, descriptor, value] if op == "conforms" => Ok(conforms(&decode(value)?, &decode(descriptor)?)),
        _ => Err("usage: horos-conform wf <descriptor> | conforms <descriptor> <value>".into()),
    }
}

fn main() -> std::process::ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    // Deep, finite oracle inputs also need room for ontos Value's recursive drop.
    let result = std::thread::Builder::new().stack_size(64 * 1024 * 1024).spawn(move || run(&args))
        .map_err(|e| e.to_string()).and_then(|t| t.join().map_err(|_| "judgment thread panicked".to_string())).and_then(|r| r);
    match result {
        Ok(v) => { println!("{v}"); std::process::ExitCode::SUCCESS }
        Err(e) => { eprintln!("{e}"); std::process::ExitCode::FAILURE }
    }
}
