//go:build js
package lbx

import (
    "bytes"
    "syscall/js"
)

/* the web page can unlock the lbx data itself (see web/index.html) and hand it
 * to the game as a Uint8Array of a zip file in the global 'momData'
 */
func browserCache() *LbxCache {
    data := js.Global().Get("momData")
    if data.IsUndefined() || data.IsNull() {
        return nil
    }

    raw := make([]byte, data.Get("length").Int())
    js.CopyBytesToGo(raw, data)

    return maybeOpenZip(bytes.NewReader(raw))
}
