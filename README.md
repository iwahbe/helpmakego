# `helpmakego` - A make dependency resolver for Go

## TODO

### Before release

- [x] Change the name from `gomakeit` to `helpmakego`.
- [x] Add tests for basic file traversal
- [ ] Add tests for partial file traversal (building a binary that only needs part of a module)
- [x] Add tests that embeds are correctly expanded.
- [ ] Add a description of how it works to the README.md (not the first thing)
- [ ] Add a example of how to use it to the README.md (that should be the first thing)
- [ ] Add a little icon to the README.md (AI generated gopher pushing a Make logo)
- [x] Validate with a complicated Pulumi package (like pulumi/pulumi-aws)
      GCP works, as long as we patch tfgen to not re-write input files (like `bridge-metadata.json`).
- [x] Add a `--test` flag, to also include test files
- [x] Validate that the test flags work as expected
- [ ] Account for `go.work` in dependency resolution
- [x] Include `go.mod` and `go.sum`
- [ ] Add a test for `replace`.
  - [ ] Ensure that this works as expected for nested `go.mod` files
  - [ ] Ensure that this works as expected for non-nested `go.mod` files

### After Release

- [ ] Add CI
  - [ ] Run tests
  - [ ] Run linter
