// Package compatrust contains backward-compatibility tests for WASM plugins
// built with the Rust plugin SDK (the test-plugins project, built separately
// from the panel). The gzipped plugin fixtures live under testdata/ and are
// read at test runtime, so the package compiles without them; a missing
// fixture fails the test that requires it.
//
// The package is part of a CI compatibility matrix: it is overlaid onto
// checkouts of older panel releases (v4.4.1, v4.3.5). Files named *_v44_*
// require the panel 4.4+ SDK modules (authz, rbac, scheduler, net) and are
// not copied onto v4.3.5; compat_v43only_test.go is copied only onto v4.3.5.
package compatrust
