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
	"sync"
)

func sortedKeys[T any](data map[string]T) []string {
	result := make([]string, len(data))
	i := 0
	for s := range data {
		result[i] = s
		i++
	}
	sort.Strings(result)
	return result
}

// Resolving a declaration means reading and parsing a source file, which is
// far too expensive to repeat on every conversion. Both layers are cached:
// function pointers and parsed files are stable for the life of the process,
// so entries never need invalidating. Misses are cached too, otherwise an
// unresolvable pointer would re-read the file every time.
var (
	astMutex    sync.Mutex
	parsedFiles = make(map[string]*parsedFile)
	funcDecls   = make(map[uintptr]*ast.FuncDecl)
)

type parsedFile struct {
	file    *ast.File
	fileSet *token.FileSet
}

func findFunction(pointer uintptr) *ast.FuncDecl {
	astMutex.Lock()
	defer astMutex.Unlock()

	if found, ok := funcDecls[pointer]; ok {
		return found
	}

	decl := resolveFunction(pointer)
	funcDecls[pointer] = decl
	return decl
}

func resolveFunction(pointer uintptr) *ast.FuncDecl {
	pc := runtime.FuncForPC(pointer)
	if pc == nil {
		return nil
	}

	splitDef := strings.Split(path.Base(pc.Name()), ".")
	valueName := splitDef[len(splitDef)-1]

	filePath, lineNumber := pc.FileLine(pointer)

	parsed := parseFile(filePath)
	if parsed == nil {
		return nil
	}

	for _, decl := range parsed.file.Decls {
		castDecl, ok := decl.(*ast.FuncDecl)
		if !ok || castDecl.Name.Name != valueName || castDecl.Type.Params == nil {
			continue
		}

		pos := parsed.fileSet.Position(castDecl.Pos())
		if pos.Line == lineNumber || pos.Line == lineNumber-1 {
			return castDecl
		}
	}

	return nil
}

func parseFile(filePath string) *parsedFile {
	if found, ok := parsedFiles[filePath]; ok {
		return found
	}

	// Cache the failure as well, so an unreadable path is only attempted once.
	parsedFiles[filePath] = nil

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	fileSet := token.NewFileSet()
	f, err := parser.ParseFile(fileSet, filePath, string(fileData), parser.ParseComments)
	if err != nil {
		return nil
	}

	parsed := &parsedFile{file: f, fileSet: fileSet}
	parsedFiles[filePath] = parsed

	return parsed
}
