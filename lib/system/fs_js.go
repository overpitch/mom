//go:build js
package system

import (
    "syscall/js"
    "bytes"
    "io"
    iofs "io/fs"
    "log"
)

/* If the web page provides a 'momSaves' object (see web/index.html) then saved
 * games are also written to the browser's storage, so they survive closing the
 * page. This matters on devices that can't download files, such as an xbox.
 *   momSaves.names() -> array of file names
 *   momSaves.get(name) -> Uint8Array
 *   momSaves.put(name, Uint8Array)
 *   momSaves.remove(name)
 */
type JsFS struct {
    *FS
    saves js.Value
}

// all game loaders share one filesystem so they see each others saves
var sharedFS *JsFS

func (fs *JsFS) persistent() bool {
    return !fs.saves.IsUndefined() && !fs.saves.IsNull()
}

// copy the saves in browser storage into the in-memory filesystem
func (fs *JsFS) restore() {
    names := fs.saves.Call("names")
    for i := range names.Length() {
        name := names.Index(i).String()
        data := fs.saves.Call("get", name)
        if data.IsUndefined() || data.IsNull() {
            continue
        }

        raw := make([]byte, data.Get("length").Int())
        js.CopyBytesToGo(raw, data)

        err := fs.FS.WriteFile(name, raw, 0644)
        if err != nil {
            log.Printf("Unable to restore saved file %v: %v", name, err)
        }
    }
}

type persistentWriter struct {
    io.WriteCloser
    fs *JsFS
    name string
}

func (writer *persistentWriter) Close() error {
    err := writer.WriteCloser.Close()
    if err != nil {
        return err
    }

    data, err := iofs.ReadFile(writer.fs.FS, writer.name)
    if err != nil {
        return err
    }

    uint8Array := js.Global().Get("Uint8Array").New(len(data))
    js.CopyBytesToJS(uint8Array, data)
    writer.fs.saves.Call("put", writer.name, uint8Array)

    return nil
}

func (fs *JsFS) Create(name string) (io.WriteCloser, error) {
    writer, err := fs.FS.Create(name)
    if err != nil || !fs.persistent() {
        return writer, err
    }

    return &persistentWriter{WriteCloser: writer, fs: fs, name: name}, nil
}

func (fs *JsFS) Remove(name string) error {
    if fs.persistent() {
        fs.saves.Call("remove", name)
    }
    return fs.FS.Remove(name)
}

func (fs *JsFS) MaybeDownload(path string) {
    // the save is already kept in browser storage
    if fs.persistent() {
        return
    }

    file, err := fs.Open(path)
    if err != nil {
        return
    }

    var data bytes.Buffer
    io.Copy(&data, file)

    file.Close()

    // Create Uint8Array
    uint8Array := js.Global().Get("Uint8Array").New(data.Len())
    if uint8Array.IsUndefined() || uint8Array.IsNull() {
        return
    }
    js.CopyBytesToJS(uint8Array, data.Bytes())

    // Create Blob
    blob := js.Global().Get("Blob").New(
        []any{uint8Array},
        map[string]any{"type": "application/octet-stream"},
    )

    // Create object URL
    url := js.Global().Get("URL").Call("createObjectURL", blob)

    // Create <a download>
    document := js.Global().Get("document")
    a := document.Call("createElement", "a")
    a.Set("href", url)
    a.Set("download", path)

    // Trigger download
    document.Get("body").Call("appendChild", a)
    a.Call("click")
    document.Get("body").Call("removeChild", a)

    // Cleanup
    js.Global().Get("URL").Call("revokeObjectURL", url)
}

var _ WriteableFS = (*JsFS)(nil)

func MakeFS() WriteableFS {
    if sharedFS == nil {
        sharedFS = &JsFS{
            FS: NewMemFS(),
            saves: js.Global().Get("momSaves"),
        }

        if sharedFS.persistent() {
            sharedFS.restore()
        }
    }

    return sharedFS
}
