package checker

import (
	"strconv"
	"strings"

	"github.com/dlclark/regexp2/v2"
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

// OH checker.ts::shouldCheckThrows through getThrowsErrorNode. State belongs
// to the checker shard, not shared AST nodes. Only declarations with a throws
// tag allocate this service or enter its version/SDK checks.
type throwsChecker struct {
	options *core.CompilerOptions
	sdk     *regexp2.Regexp
	since   []*regexp2.Regexp
	checked map[*ast.Node]bool
}

func (c *Checker) checkThrowsCall(node, declaration *ast.Node) {
	if !ast.IsCallExpression(node) || declaration == nil ||
		!(ast.IsMethodDeclaration(declaration) || declaration.Kind == ast.KindMethodSignature || ast.IsFunctionDeclaration(declaration)) {
		return
	}
	useFile := ast.GetSourceFileOfNode(node)
	if useFile.ScriptKind != core.ScriptKindETS && !c.compilerOptions.UsesOHModuleResolution() {
		return
	}
	tags := throwsTags(declaration)
	if len(tags) == 0 {
		return
	}
	if c.throws == nil {
		pattern := "" // new RegExp(undefined) in OH matches every path.
		if path := c.compilerOptions.EtsLoaderPath; path != "" {
			parent := "../../../.."
			if strings.HasSuffix(tspath.NormalizePath(path), "dynamic/build-tools/ets-loader") {
				parent = "../../../../.."
			}
			pattern = tspath.GetNormalizedAbsolutePath(parent, path)
		}
		c.throws = &throwsChecker{
			options: c.compilerOptions,
			// OH deliberately constructs a RegExp from the unescaped SDK path.
			// RE2 or literal path containment would change that contract.
			sdk:     regexp2.MustCompile(pattern, regexp2.ECMAScript),
			checked: make(map[*ast.Node]bool),
		}
	}
	if !c.throws.shouldCheck(declaration, tags) || throwsHandled(node) {
		return
	}
	for current := node.Parent; current != nil; current = current.Parent {
		if ast.IsFunctionLikeDeclaration(current) {
			if !(ast.IsFunctionDeclaration(current) || ast.IsMethodDeclaration(current)) || len(throwsTags(current)) != 0 {
				return
			}
			errorNode := node
			switch expression := node.Expression(); expression.Kind {
			case ast.KindIdentifier, ast.KindElementAccessExpression:
				errorNode = expression
			case ast.KindPropertyAccessExpression:
				errorNode = expression.Name()
			}
			c.error(errorNode, diagnostics.Function_may_throw_exceptions_Special_handling_is_required)
			return
		}
	}
}

// OH getJSDocTags collects the last JSDoc at each owning location, not only
// the first location with tags. Throws has no special @type ownership filter.
func throwsTags(node *ast.Node) []*ast.Node {
	var result []*ast.Node
	for current := node; current != nil && current.Parent != nil; current = ast.GetNextJSDocCommentLocation(current) {
		docs := current.JSDoc(nil)
		if len(docs) == 0 || docs[len(docs)-1].AsJSDoc().Tags == nil {
			continue
		}
		for _, tag := range docs[len(docs)-1].AsJSDoc().Tags.Nodes {
			if tag.TagName().Text() == "throws" {
				result = append(result, tag)
			}
		}
	}
	return result
}

func (t *throwsChecker) shouldCheck(declaration *ast.Node, tags []*ast.Node) bool {
	if result, ok := t.checked[declaration]; ok {
		return result
	}
	result := t.checkDeclaration(declaration, tags)
	t.checked[declaration] = result
	return result
}

func (t *throwsChecker) checkDeclaration(declaration *ast.Node, tags []*ast.Node) bool {
	path := tspath.NormalizePath(ast.GetSourceFileOfNode(declaration).FileName())
	for _, excluded := range []string{"/node_modules/", "/oh_modules/", "/js_util_module/"} {
		if strings.Contains(path, excluded) {
			return false
		}
	}
	for _, parameter := range declaration.Parameters() {
		if typ := parameter.Type(); typ != nil && ast.IsTypeReferenceNode(typ) && ast.IsIdentifier(typ.AsTypeReferenceNode().TypeName) {
			name := typ.AsTypeReferenceNode().TypeName.Text()
			if name == "AsyncCallback" || name == "ErrorCallback" {
				return false
			}
		}
	}
	sdk, err := t.sdk.MatchString(path)
	if err != nil {
		panic(err)
	}
	for _, tag := range tags {
		if !sdk || !t.skipTag(tag) {
			return true
		}
	}
	return false
}

func (t *throwsChecker) skipTag(tag *ast.Node) bool {
	comment := rawThrowsTagComment(tag)
	if strings.Contains(comment, "{ BusinessError } 401 -") {
		return true
	}
	if t.options.CompileSdkVersion == nil {
		return false
	}
	if t.since == nil {
		for _, pattern := range []string{
			`\[since\s+(?:\d+\.\d+\.\d+\((\d+)\))(?:\s*-\s*(?:\d+\.\d+\.\d+\((\d+)\)))?\]`,
			`\[since\s+(?:\d+\.\d+\.\d+\((\d+)\))\s*-\s*(\d+)\.\d+\.\d+\]`,
			`\[since\s+(\d+)\s*-\s*(\d+)\.\d+\.\d+\]`,
			`\[since\s+(\d+)\.\d+\.\d+(?:\s*-\s*(\d+)\.\d+\.\d+)?\]`,
			`\[since\s+(\d+)(?:\s*-\s*(\d+))?\]`,
		} {
			t.since = append(t.since, regexp2.MustCompile(pattern, regexp2.ECMAScript))
		}
	}
	version := *t.options.CompileSdkVersion
	for _, pattern := range t.since {
		match, err := pattern.FindStringMatch(comment)
		if err != nil {
			panic(err)
		}
		if match == nil {
			continue
		}
		from, _ := strconv.ParseFloat(match.GroupByNumber(1).String(), 64)
		toText := match.GroupByNumber(2).String()
		if toText == "" {
			return !(version >= from)
		}
		to, _ := strconv.ParseFloat(toText, 64)
		return !(version >= min(from, to) && version <= max(from, to))
	}
	return false
}

// The OH parser version treats @throws as an unknown tag and therefore keeps
// the complete text after the tag name in `comment`. Current TypeScript parses
// the optional type expression separately. Read the original source span so
// spacing-sensitive OH checks observe the same text without removing modern
// JSDocThrowsTag support.
func rawThrowsTagComment(tag *ast.Node) string {
	file := ast.GetSourceFileOfNode(tag)
	if file != nil && tag.Pos() >= 0 && tag.End() <= len(file.Text()) && tag.Pos() <= tag.End() {
		text := file.Text()[tag.Pos():tag.End()]
		if index := strings.Index(text, "@throws"); index >= 0 {
			return strings.TrimSpace(text[index+len("@throws"):])
		}
	}
	var text strings.Builder
	if comments := tag.CommentList(); comments != nil {
		for i, comment := range comments.Nodes {
			if i != 0 {
				text.WriteByte('\n')
			}
			text.WriteString(comment.Text())
		}
	}
	return text.String()
}

func throwsHandled(node *ast.Node) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if ast.IsMethodDeclaration(current) || ast.IsFunctionDeclaration(current) {
			return false
		}
		if ast.IsTryStatement(current) {
			statement := current.AsTryStatement()
			if statement.TryBlock != nil && statement.CatchClause != nil && node.Pos() >= statement.TryBlock.Pos() && node.End() <= statement.TryBlock.End() {
				return true
			}
		}
		if ast.IsPropertyAccessExpression(current) && current.Name().Text() == "catch" && current.Parent != nil && ast.IsCallExpression(current.Parent) {
			return true
		}
	}
	return false
}
