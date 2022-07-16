package crystalline

import "syscall/js"

func setNamespace(appName string, packageName string, name string, value interface{}) {
	goNs := js.Global().Get("go")
	if goNs.IsUndefined() {
		js.Global().Set("go", make(map[string]interface{}))
		goNs = js.Global().Get("go")
	}

	appNs := goNs.Get(appName)
	if appNs.IsUndefined() {
		goNs.Set(appName, make(map[string]interface{}))
		appNs = goNs.Get(appName)
	}

	pkgNs := appNs.Get(packageName)
	if pkgNs.IsUndefined() {
		appNs.Set(packageName, make(map[string]interface{}))
		pkgNs = appNs.Get(packageName)
	}

	pkgNs.Set(name, value)
}
