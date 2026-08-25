;; importing is a hand-written plugin module for the host-import and
;; host-call observation tests. It satisfies the PluginService ABI (packed
;; ptr<<32|len results, malloc/free) and imports three host functions: two
;; are gated (nodecmd execute_command, nodefs read_dir), one is open
;; (servers find_servers). handle_event calls execute_command once with an
;; empty request, so a host-call observer sees exactly one nodecmd call.
;; Rebuild with: wat2wasm importing.wat -o importing.wasm
(module
  (import "gameap-nodecmd" "execute_command" (func $execute_command (param i32 i32) (result i64)))
  (import "gameap-nodefs" "read_dir" (func $read_dir (param i32 i32) (result i64)))
  (import "gameap-servers" "find_servers" (func $find_servers (param i32 i32) (result i64)))

  (memory (export "memory") 1)
  (global $heap (mut i32) (i32.const 1024))

  ;; PluginInfo{id: "importing", api_version: "1"}, protobuf wire format.
  (data (i32.const 0) "\0a\09importing\4a\011")

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
    (call $pack (i32.const 0) (i32.const 14)))

  (func $empty (param i32 i32) (result i64)
    (i64.const 0))
  (export "plugin_service_initialize" (func $empty))
  (export "plugin_service_shutdown" (func $empty))
  (export "plugin_service_get_subscribed_events" (func $empty))
  (export "plugin_service_get_http_routes" (func $empty))
  (export "plugin_service_handle_http_request" (func $empty))

  ;; Calls execute_command with an empty request and answers an empty
  ;; EventResult; the imports that are never called still appear in the
  ;; import section.
  (func (export "plugin_service_handle_event") (param i32 i32) (result i64)
    (drop (call $execute_command (i32.const 0) (i32.const 0)))
    (i64.const 0))

  (func $unused (export "unused_nodefs") (param i32 i32) (result i64)
    (call $read_dir (local.get 0) (local.get 1)))

  (func $unused2 (export "unused_servers") (param i32 i32) (result i64)
    (call $find_servers (local.get 0) (local.get 1)))
)
