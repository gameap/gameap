# mergefs

Read-only, layered `fs.FS`. Several filesystems are stacked; a lookup resolves to the **first layer that
contains the name**, so earlier layers shadow later ones. Directory listings are the **union** of every
layer that holds the directory, deduplicated by name (earlier layers win) and sorted.

Used to let plugins contribute translation (`/lang/*.json`) and frontend static files that layer over
GameAP's built-in `embed.FS`. Plugin layers are placed **above** the base layer, so a plugin file shadows a
core file of the same path, and a plugin file with a new name simply adds it.

## Usage

```go
// Static layers (earlier wins):
fsys := mergefs.New(pluginFS, baseFS)

// Dynamic layers, re-evaluated on every operation — reflects plugins loaded at
// runtime without rebuilding:
fsys := mergefs.NewDynamic(func() []fs.FS {
    layers := collectEnabledPluginLayers() // above base
    return append(layers, baseFS)          // base last
})

// Build an in-memory layer from a plugin's returned files:
pluginFS, err := mergefs.FromFiles(map[string][]byte{
    "es.json":              esBytes,
    "assets/plugin/app.js": appBytes,
})
```

## Guarantees

- **Seekable files.** `Open` returns the underlying layer's file unchanged, so files that implement
  `io.ReadSeeker` (`embed.FS`, `FromFiles`) stay seekable — required by `http.ServeContent` /
  `http.FileServer`.
- **`fs` interfaces.** Implements `fs.FS`, `fs.StatFS`, `fs.ReadFileFS`, `fs.ReadDirFS`. `FromFiles` output
  additionally passes `testing/fstest.TestFS`.
- **Path safety.** `FromFiles` rejects any key that is not a valid `fs` path (`fs.ValidPath`): no absolute
  paths, no `.`/`..` elements.
- **Kind resolution.** The first layer to hold a name decides whether it is a file or a directory; a file in
  an earlier layer shadows a directory of the same name in a later one, and vice versa.

The filesystems are read-only. There is no `Create`/`Write`; contributions are supplied as fixed layers.
