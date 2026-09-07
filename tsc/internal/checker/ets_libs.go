package checker

import (
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

// OH utilities.ts::getEtsLibs includes configured paths and exactly one level
// of triple-slash references. Snapshot the list once per immutable Program,
// rather than scanning every source file on each global symbol lookup.
func (c *Checker) initializeEtsLibFiles() {
	if c.compilerOptions.Ets.Libs.IsZero() {
		return
	}
	c.etsLibFiles = make(map[string]struct{})
	for name := range c.compilerOptions.Ets.Libs.Values() {
		c.etsLibFiles[tspath.GetNormalizedAbsolutePath(name, c.program.GetCurrentDirectory())] = struct{}{}
	}
	var references []string
	for _, file := range c.files {
		if _, ok := c.etsLibFiles[tspath.GetNormalizedAbsolutePath(file.FileName(), c.program.GetCurrentDirectory())]; !ok {
			continue
		}
		for _, ref := range file.ReferencedFiles {
			references = append(references, tspath.GetNormalizedAbsolutePath(ref.FileName, tspath.GetDirectoryPath(file.FileName())))
		}
	}
	for _, name := range references {
		c.etsLibFiles[name] = struct{}{}
	}
}

// OH checker.ts::isValidFromLibs is applied only when a lookup receives a
// source location (global lookup). Local/namespace lookup remains unchanged.
func (c *Checker) isValidFromEtsLibs(symbol *ast.Symbol, node *ast.Node) bool {
	if node == nil || len(symbol.Declarations) == 0 || len(c.etsLibFiles) == 0 {
		return true
	}
	fileName := strings.TrimSpace(ast.GetSourceFileOfNode(node).FileName())
	if fileName == "" {
		return true
	}
	fileName = tspath.GetNormalizedAbsolutePath(fileName, c.program.GetCurrentDirectory())
	if strings.HasSuffix(fileName, tspath.ExtensionEts) {
		return true
	}
	if _, ok := c.etsLibFiles[fileName]; ok {
		return true
	}
	for _, declaration := range symbol.Declarations {
		name := ast.GetSourceFileOfNode(declaration).FileName()
		if name == "" {
			return true
		}
		if _, ok := c.etsLibFiles[tspath.GetNormalizedAbsolutePath(name, c.program.GetCurrentDirectory())]; !ok {
			return true
		}
	}
	return false
}
