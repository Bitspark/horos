"""Exercise installed packages with fresh caches and no registry or Git credentials."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import tarfile

from package import ROOT, VERSION, digest, receipt


def run(args, cwd, env, retry=False):
    for attempt in range(30 if retry else 1):
        result = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True)
        if result.returncode == 0:
            print(result.stdout)
            return result.stdout
        if not retry or attempt == 29:
            raise RuntimeError(f"{args[0]} failed ({result.returncode})\n{result.stdout}\n{result.stderr}")
        time.sleep(10)
    raise AssertionError("unreachable")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["candidate", "published"])
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    directory = args.directory.resolve()
    accepted = receipt(directory)
    with tempfile.TemporaryDirectory(prefix="horos-consumer-") as temp:
        root = Path(temp)
        # The verify job receives no publishing credentials. Also scrub inherited config
        # for local reproductions: dependencies must resolve as an anonymous consumer.
        env = {key: value for key, value in os.environ.items() if not any(s in key.upper() for s in ("TOKEN", "SECRET", "PASSWORD")) and not key.upper().startswith(("NPM_CONFIG_", "CARGO_REGISTRIES_", "GIT_CONFIG_"))}
        empty = root / "empty-config"
        empty.write_text("")
        env.update({"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": str(empty), "GIT_TERMINAL_PROMPT": "0",
                    "npm_config_userconfig": str(empty), "npm_config_globalconfig": str(root / "empty-global"),
                    "npm_config_cache": str(root / "npm-cache"), "npm_config_registry": "https://registry.npmjs.org/",
                    "CARGO_HOME": str(root / "cargo-home"), "CARGO_TARGET_DIR": str(root / "cargo-target"),
                    "GOPROXY": "https://proxy.golang.org", "GOSUMDB": "sum.golang.org", "GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "",
                    "GOMODCACHE": str(root / "go-modcache"), "GOCACHE": str(root / "go-buildcache"), "GOWORK": "off", "GOFLAGS": ""})
        (root / "empty-global").write_text("")
        shutil.copyfile(ROOT / "vectors/conformance.json", root / "conformance.json")
        npm = root / "npm"
        npm.mkdir()
        (npm / "package.json").write_text('{"name":"horos-consumer","private":true,"type":"module"}')
        target = f"@bitspark/horos@{VERSION}" if args.mode == "published" else str(directory / "bitspark-horos-0.6.0.tgz")
        run(["npm", "install", "--ignore-scripts", "--no-audit", "--no-fund", target, "typescript@5.7.3"], npm, env, args.mode == "published")
        (npm / "check.mjs").write_text('''import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {decode} from "@bitspark/ontos-codec";
import {wf, conforms} from "@bitspark/horos";
import {diagnose, diagnoseWf, completedBool} from "@bitspark/horos/diagnose";
const doc=JSON.parse(readFileSync("../conformance.json","utf8"));
for(const c of doc.cases) {
  const t=decode(Buffer.from(c.descriptor,"hex"));
  assert.equal(wf(t),c.wf,c.name);
  const w=completedBool(diagnoseWf(t)); if(w!==undefined) assert.equal(w,c.wf,c.name);
  for(const item of c.values) {
    const v=decode(Buffer.from(item.value,"hex"));
    assert.equal(conforms(v,t),item.conforms,c.name);
    const b=completedBool(diagnose(v,t)); if(b!==undefined) assert.equal(b,item.conforms,c.name);
  }
}
for(const path of ["semantics","wire"]) await assert.rejects(import("@bitspark/horos/"+path), {code:"ERR_PACKAGE_PATH_NOT_EXPORTED"});
console.log(`npm: ${doc.cases.length} oracle descriptors and diagnostics from installed package`);
''')
        (npm / "check.ts").write_text('''import {wf, conforms} from "@bitspark/horos";
import {diagnose, type Diagnosis} from "@bitspark/horos/diagnose";
import {tuple} from "@bitspark/ontos-core";
const value=tuple([]); const result:Diagnosis=diagnose(value,value);
void [wf(value), conforms(value,value),result];
''')
        run(["node", "check.mjs"], npm, env)
        run(["node", "node_modules/typescript/bin/tsc", "--noEmit", "--strict", "--target", "ES2022", "--module", "NodeNext", "--moduleResolution", "NodeNext", "check.ts"], npm, env)
        if args.mode == "published":
            run(["npm", "audit", "signatures"], npm, env)

        rust = root / "rust"
        (rust / "src").mkdir(parents=True)
        dependency = f'bitspark-horos = "={VERSION}"'
        if args.mode == "candidate":
            extracted = root / "crate"
            with tarfile.open(directory / "bitspark-horos-0.6.0.crate") as archive:
                archive.extractall(extracted, filter="data")
            run(["cargo", "test", "--locked"], extracted / "bitspark-horos-0.6.0", env)
            dependency = 'bitspark-horos = { path = ' + json.dumps(str(extracted / "bitspark-horos-0.6.0")) + ' }'
        (rust / "Cargo.toml").write_text('[package]\nname="horos-consumer"\nversion="0.0.0"\nedition="2024"\n[dependencies]\n' + dependency + '\nontos-codec={package="bitspark-ontos-codec",version="=0.15.0"}\nserde_json="1"\n')
        (rust / "src/main.rs").write_text('''fn main() {
    std::thread::Builder::new().stack_size(64*1024*1024).spawn(|| {
        fn decode(s:&str)->ontos_codec::Value { unreachable!() }
    }).unwrap().join().unwrap();
}
'''.replace('''        fn decode(s:&str)->ontos_codec::Value { unreachable!() }''', '''        let doc:serde_json::Value=serde_json::from_str(&std::fs::read_to_string("../conformance.json").unwrap()).unwrap();
        let bytes=|s:&str| (0..s.len()).step_by(2).map(|i|u8::from_str_radix(&s[i..i+2],16).unwrap()).collect::<Vec<_>>();
        for c in doc["cases"].as_array().unwrap() {
            let t=ontos_codec::decode(&bytes(c["descriptor"].as_str().unwrap())).unwrap();
            assert_eq!(horos::wf(&t),c["wf"].as_bool().unwrap());
            if let Some(b)=horos::diagnose::diagnose_wf(&t).as_completed_bool() { assert_eq!(b,horos::wf(&t)); }
            for item in c["values"].as_array().unwrap() {
                let v=ontos_codec::decode(&bytes(item["value"].as_str().unwrap())).unwrap();
                assert_eq!(horos::conforms(&v,&t),item["conforms"].as_bool().unwrap());
                if let Some(b)=horos::diagnose::diagnose(&v,&t).as_completed_bool() { assert_eq!(b,horos::conforms(&v,&t)); }
            }
        }
        println!("Cargo: installed public API and complete oracle passed");'''))
        run(["cargo", "generate-lockfile"], rust, env, args.mode == "published")
        run(["cargo", "run", "--locked"], rust, env)

        go_origin = None
        if args.mode == "published":
            go = root / "go"
            go.mkdir()
            (go / "go.mod").write_text("module consumer\n\ngo 1.25\n")
            meta = json.loads(run(["go", "mod", "download", "-json", "github.com/Bitspark/horos/core/go@v0.6.0"], go, env, True))
            assert not meta.get("Error"), meta
            go_origin = meta["Origin"]
            assert go_origin["Hash"] == accepted["source"], "Go proxy tag/source mismatch"
            # Tests ship inside the module, including its independently authored oracle.
            assert digest(Path(meta["Dir"]) / "testdata/conformance.json") == accepted["oracle_sha256"]
            run(["go", "test", "-mod=readonly", "-race", "-count=1", "./..."], Path(meta["Dir"]), env)

        verification = {"source": accepted["source"], "version": VERSION, "mode": args.mode,
                        "anonymous_consumers": ["npm", "crates.io"] + (["Go proxy"] if go_origin else []),
                        "oracle_sha256": accepted["oracle_sha256"], "go_origin": go_origin,
                        "ci": os.environ.get("ACCEPTED_CI"), "run_id": os.environ.get("GITHUB_RUN_ID")}
        (directory / f"{args.mode}-verification.json").write_text(json.dumps(verification, indent=2) + "\n")


if __name__ == "__main__":
    main()
