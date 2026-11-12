# Fuzz seed module sets

Each `.ontos` file is the ontos-codec-v1 encoding of `Tuple(root, dependency...)`:
one module set that an existing test in this package passed to `Resolve` or
`ResolveWithLimits`. The fuzz targets in `fuzz_test.go` add every file as a
seed, together with the bundled `horos.kinds` asset.

They were harvested on 11 November 2026 from main `3ba6b65` by temporarily
adding a hook at the top of `ResolveWithLimits` that wrote each call's encoded
module set here (named by the first eight bytes of its SHA-256), running
`go test ./declarations/`, and removing the hook. The hook skipped calls whose
root or dependencies were not encodable values (the tests' nil inputs). Files
are frozen inputs: when the test modules change, the old files remain valid
seeds, and a new harvest may add files.

Inputs that a fuzzing run found failing are kept separately, by the Go
toolchain, under `testdata/fuzz/<target>/`.
