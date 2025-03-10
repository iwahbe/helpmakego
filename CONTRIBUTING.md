# Contributing to `helpmakego`

## Building

`make build` will do everything, and put the resulting binary into `bin/helpmakego`.

## Tests

`make test` will run all tests.

## Benchmarks

Developers are busy people, and speed is *critical* to keeping `helpmakego` out of their way.

Any change to the hot loop of finding packages must come with benchmark results showing
that it doesn't decrease performance for users.

I like to use [`hyperfine`]() for checking performance. Running `make benchmark` will
generate a benchmark report comparing the local build of `helpmakego` with the version
currently on master.
