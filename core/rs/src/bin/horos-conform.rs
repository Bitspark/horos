//! Evaluate wf/conforms, and their diagnoses with locations, over ontos-codec-v1 hex.
use horos::diagnose::{diagnose_located, diagnose_wf_located, Diagnosis, Located, Step};
use horos::{conforms, wf};
use ontos_core::Value;

const USAGE: &str = "usage: horos-conform wf <descriptor> | conforms <descriptor> <value> \
                     | diagnose-wf <descriptor> | diagnose <descriptor> <value>";

fn decode(hex: &str) -> Result<Value, String> {
    if !hex.len().is_multiple_of(2)
        || !hex
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    {
        return Err("invalid lowercase hex".into());
    }
    let bytes: Result<Vec<_>, _> = (0..hex.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(&hex[i..i + 2], 16))
        .collect();
    ontos_codec::decode(&bytes.map_err(|e| e.to_string())?).map_err(|e| format!("{e:?}"))
}

/// A JSON string literal: quote, backslash and control characters escaped.
fn json_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

/// The form contract's JSON rendering of a located diagnosis.
fn render(d: &Located) -> String {
    let verdict = match d.diagnosis {
        Diagnosis::Conforms => return r#"{"verdict":"conforms"}"#.into(),
        Diagnosis::Mismatch => r#""verdict":"mismatch""#,
        Diagnosis::NotWellFormed => r#""verdict":"not_well_formed""#,
        Diagnosis::Exhausted(_) => r#""verdict":"exhausted","limit":"recursion_depth""#,
    };
    let Some(loc) = &d.location else {
        return format!("{{{verdict}}}");
    };
    let path: Vec<String> = loc
        .path
        .iter()
        .map(|s| match s {
            Step::Item(i) => format!(r#"{{"kind":"item","index":{i}}}"#),
            Step::Elem(i) => format!(r#"{{"kind":"elem","index":{i}}}"#),
            Step::Key(i) => format!(r#"{{"kind":"key","index":{i}}}"#),
            Step::Value(i) => format!(r#"{{"kind":"value","index":{i}}}"#),
            Step::Field(n) => format!(r#"{{"kind":"field","name":{}}}"#, json_string(n)),
        })
        .collect();
    let at: Vec<String> = loc.at.iter().map(usize::to_string).collect();
    let mut l = format!(
        r#"{{"path":[{}],"at":[{}],"reason":"{}""#,
        path.join(","),
        at.join(","),
        loc.reason.as_str()
    );
    if let Some(f) = &loc.field {
        l.push_str(&format!(r#","field":{}"#, json_string(f)));
    }
    if let Some(a) = loc.arg {
        l.push_str(&format!(r#","arg":{a}"#));
    }
    format!("{{{verdict},\"location\":{l}}}}}")
}

fn run(args: &[String]) -> Result<String, String> {
    match args {
        [op, descriptor] if op == "wf" => Ok(wf(&decode(descriptor)?).to_string()),
        [op, descriptor, value] if op == "conforms" => {
            Ok(conforms(&decode(value)?, &decode(descriptor)?).to_string())
        }
        [op, descriptor] if op == "diagnose-wf" => {
            Ok(render(&diagnose_wf_located(&decode(descriptor)?)))
        }
        [op, descriptor, value] if op == "diagnose" => Ok(render(&diagnose_located(
            &decode(value)?,
            &decode(descriptor)?,
        ))),
        _ => Err(USAGE.into()),
    }
}

fn main() -> std::process::ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    // Deep, finite oracle inputs also need room for ontos Value's recursive drop.
    let result = std::thread::Builder::new()
        .stack_size(64 * 1024 * 1024)
        .spawn(move || run(&args))
        .map_err(|e| e.to_string())
        .and_then(|t| t.join().map_err(|_| "judgment thread panicked".to_string()))
        .and_then(|r| r);
    match result {
        Ok(v) => {
            println!("{v}");
            std::process::ExitCode::SUCCESS
        }
        Err(e) => {
            eprintln!("{e}");
            std::process::ExitCode::FAILURE
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn from_hex(s: &str) -> Value {
        decode(s).unwrap_or_else(|e| panic!("bad corpus hex {s}: {e}"))
    }

    /// Replays the hand-authored location corpus (vectors/diagnose.json, ADR 0009) through
    /// the rendering this command prints, comparing parsed JSON with the expectations.
    #[test]
    fn the_location_corpus_replays() {
        std::thread::Builder::new()
            .stack_size(64 * 1024 * 1024)
            .spawn(|| {
                let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
                    .join("testdata")
                    .join("diagnose.json");
                let text = std::fs::read_to_string(&path).unwrap_or_else(|e| {
                    panic!("the location corpus {} is required: {e}", path.display())
                });
                let doc: serde_json::Value = serde_json::from_str(&text).unwrap();
                let parse = |s: String| -> serde_json::Value {
                    serde_json::from_str(&s).unwrap_or_else(|e| panic!("{s}: {e}"))
                };
                let mut checked = 0usize;
                for c in doc["cases"].as_array().expect("cases") {
                    let name = c["name"].as_str().unwrap();
                    let t = from_hex(c["descriptor"].as_str().unwrap());
                    let got = parse(render(&diagnose_wf_located(&t)));
                    assert_eq!(got, c["wf"], "diagnose-wf for {name}");
                    checked += 1;
                    for (i, val) in c["values"].as_array().unwrap().iter().enumerate() {
                        let v = from_hex(val["value"].as_str().unwrap());
                        let got = parse(render(&diagnose_located(&v, &t)));
                        assert_eq!(got, val["diagnosis"], "diagnose for {name} / value[{i}]");
                        checked += 1;
                    }
                }
                assert!(checked > 0, "the location corpus has no judgments");
            })
            .expect("spawn")
            .join()
            .expect("corpus replay");
    }

    #[test]
    fn strings_are_escaped() {
        assert_eq!(
            json_string("a\"b\\c\n\u{1}é"),
            "\"a\\\"b\\\\c\\u000a\\u0001é\""
        );
    }
}
