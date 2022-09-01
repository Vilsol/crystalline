package crystalline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
)

func SortedKeys[T any](data map[string]T) []string {
	result := make([]string, len(data))
	i := 0
	for s := range data {
		result[i] = s
		i++
	}
	sort.Strings(result)
	return result
}

func findFunction(pointer uintptr) *ast.FuncDecl {
	pc := runtime.FuncForPC(pointer)

	splitDef := strings.Split(path.Base(pc.Name()), ".")
	valueName := splitDef[len(splitDef)-1]

	filePath, lineNumber := pc.FileLine(pointer)

	fileData, err := os.ReadFile(filePath)
	if err == nil {
		fileSet := token.NewFileSet()
		f, err := parser.ParseFile(fileSet, filePath, string(fileData), parser.ParseComments)
		if err == nil {
			for _, decl := range f.Decls {
				switch castDecl := decl.(type) {
				case *ast.FuncDecl:
					if castDecl.Name.Name == valueName && castDecl.Type.Params != nil {
						pos := fileSet.Position(castDecl.Pos())
						if pos.Line == lineNumber || pos.Line == lineNumber-1 {
							return castDecl
						}
					}
				}
			}
		}
	}
	return nil
}
