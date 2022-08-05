//go:build js

package crystalline

import "syscall/js"

var uint8ArrayConstructor js.Value

func init() {
	uint8ArrayConstructor = js.Global().Get("Uint8Array")
}

func convertByteArray(data []byte) (interface{}, error) {
	outArray := uint8ArrayConstructor.New(len(data))
	js.CopyBytesToJS(outArray, data)
	return outArray, nil
}
