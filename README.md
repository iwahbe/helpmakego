# `helpmakego` - A make dependency resolver for Go

## TODO

### Before release

- [X] Change the name from `gomakeit` to `helpmakego`.
- [ ] Add tests for basic file traversal
- [ ] Add tests for partial file traversal (building a binary that only needs part of a module)
- [ ] Add tests that embeds are correctly expanded.
- [ ] Add a description of how it works to the README.md (not the first thing)
- [ ] Add a example of how to use it to the README.md (that should be the first thing)
- [ ] Add a little icon to the README.md (AI generated gopher pushing a Make logo)
- [ ] Validate with a complicated Pulumi package (like pulumi/pulumi-aws)
- [x] Add a `--test` flag, to also include test files
- [ ] Validate that the test flags work as expected
- [ ] Account for `go.work` in dependency resolution
- [x] Include `go.mod` and `go.sum`

### After Release

- [ ] Add CI
  - [ ] Run tests
  - [ ] Run linter
