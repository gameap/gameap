;; misbehaving is a hand-written plugin module for runtime hardening tests.
;; It satisfies the PluginService ABI (packed ptr<<32|len results, malloc/free)
;; with the smallest possible bodies and misbehaves on purpose:
;;   - plugin_service_handle_http_request spins forever (call deadline tests);
;;   - plugin_service_handle_event prints a line to stdout and then terminates
;;     the module through proc_exit(3) (guest exit and guest log tests).
;; Rebuild with: wat2wasm misbehaving.wat -o misbehaving.wasm
(module
  (import "wasi_snapshot_preview1" "proc_exit" (func $proc_exit (param i32)))
  (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))

  (memory (export "memory") 1)
  (global $heap (mut i32) (i32.const 1024))

  ;; PluginInfo{id: "misbehaving", api_version: "1"}, protobuf wire format.
  (data (i32.const 0) "\0a\0bmisbehaving\4a\011")
  ;; Line printed to stdout by handle_event.
  (data (i32.const 64) "hello from guest\n")

  (func (export "malloc") (param $size i32) (result i32)
    (local $ptr i32)
    (local.set $ptr (global.get $heap))
    (global.set $heap
      (i32.and
        (i32.add (i32.add (global.get $heap) (local.get $size)) (i32.const 7))
        (i32.const -8)))
    (local.get $ptr))

  (func (export "free") (param i32))

  (func (export "plugin_service_api_version") (result i64)
    (i64.const 1))

  (func $pack (param $ptr i32) (param $len i32) (result i64)
    (i64.or
      (i64.shl (i64.extend_i32_u (local.get $ptr)) (i64.const 32))
      (i64.extend_i32_u (local.get $len))))

  (func (export "plugin_service_get_info") (param i32 i32) (result i64)
    (call $pack (i32.const 0) (i32.const 16)))

  (func $empty (param i32 i32) (result i64)
    (i64.const 0))
  (export "plugin_service_initialize" (func $empty))
  (export "plugin_service_shutdown" (func $empty))
  (export "plugin_service_get_subscribed_events" (func $empty))
  (export "plugin_service_get_http_routes" (func $empty))

  (func (export "plugin_service_handle_http_request") (param i32 i32) (result i64)
    (loop $spin (br $spin))
    (i64.const 0))

  (func (export "plugin_service_handle_event") (param i32 i32) (result i64)
    ;; iovec at 128: {base: 64, len: 17}; bytes written land at 136.
    (i32.store (i32.const 128) (i32.const 64))
    (i32.store (i32.const 132) (i32.const 17))
    (drop (call $fd_write (i32.const 1) (i32.const 128) (i32.const 1) (i32.const 136)))
    (call $proc_exit (i32.const 3))
    (i64.const 0))
)
