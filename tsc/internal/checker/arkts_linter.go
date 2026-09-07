package checker

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/jsnum"
	"github.com/microsoft/TypeScript/tsc/internal/module"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

// arkTSFault is kept in the same order as ArkTSLinter_1_1/Problems.ts. The
// numeric value is internal; diagnostics use the cookbook reference from
// arkTSFaultAttributes as their public code.
type arkTSFault int

const (
	arkTSAnyType arkTSFault = iota
	arkTSSymbolType
	arkTSObjectLiteralNoContextType
	arkTSArrayLiteralNoContextType
	arkTSComputedPropertyName
	arkTSLiteralAsPropertyName
	arkTSTypeQuery
	arkTSIsOperator
	arkTSDestructuringParameter
	arkTSYieldExpression
	arkTSInterfaceMerging
	arkTSEnumMerging
	arkTSInterfaceExtendsClass
	arkTSIndexMember
	arkTSWithStatement
	arkTSThrowStatement
	arkTSIndexedAccessType
	arkTSUnknownType
	arkTSForInStatement
	arkTSInOperator
	arkTSFunctionExpression
	arkTSIntersectionType
	arkTSObjectTypeLiteral
	arkTSCommaOperator
	arkTSLimitedReturnTypeInference
	arkTSClassExpression
	arkTSDestructuringAssignment
	arkTSDestructuringDeclaration
	arkTSVarDeclaration
	arkTSCatchWithUnsupportedType
	arkTSDeleteOperator
	arkTSDeclWithDuplicateName
	arkTSUnaryArithmNotNumber
	arkTSConstructorType
	arkTSConstructorIface
	arkTSConstructorFuncs
	arkTSCallSignature
	arkTSTypeAssertion
	arkTSPrivateIdentifier
	arkTSLocalFunction
	arkTSConditionalType
	arkTSMappedType
	arkTSNamespaceAsObject
	arkTSClassAsObject
	arkTSNonDeclarationInNamespace
	arkTSGeneratorFunction
	arkTSFunctionContainsThis
	arkTSPropertyAccessByIndex
	arkTSJsxElement
	arkTSEnumMemberNonConstInit
	arkTSImplementsClass
	arkTSMethodReassignment
	arkTSMultipleStaticBlocks
	arkTSThisType
	arkTSInterfaceExtendDifferentProperties
	arkTSStructuralIdentity
	arkTSExportAssignment
	arkTSImportAssignment
	arkTSGenericCallNoTypeArgs
	arkTSParameterProperties
	arkTSInstanceofUnsupported
	arkTSShorthandAmbientModuleDecl
	arkTSWildcardsInModuleName
	arkTSUMDModuleDefinition
	arkTSNewTarget
	arkTSDefiniteAssignment
	arkTSPrototype
	arkTSGlobalThis
	arkTSUtilityType
	arkTSPropertyDeclOnFunction
	arkTSFunctionApplyCall
	arkTSFunctionBind
	arkTSConstAssertion
	arkTSImportAssertion
	arkTSSpreadOperator
	arkTSLimitedStdLibAPI
	arkTSErrorSuppression
	arkTSStrictDiagnostic
	arkTSImportAfterStatement
	arkTSESObjectType
	arkTSSendableClassInheritance
	arkTSSendablePropType
	arkTSSendableDefiniteAssignment
	arkTSSendableGenericTypes
	arkTSSendableCapturedVars
	arkTSSendableClassDecorator
	arkTSSendableObjectInitialization
	arkTSSendableComputedPropName
	arkTSSendableAsExpression
	arkTSSharedNoSideEffectImport
	arkTSSharedModuleExports
	arkTSSharedModuleNoWildcardExport
	arkTSNoTSImportETS
	arkTSSendableTypeInheritance
	arkTSSendableTypeExported
	arkTSNoTSReExportETS
	arkTSNoNamespaceImportETSToTS
	arkTSNoSideEffectImportETSToTS
	arkTSSendableExplicitFieldType
	arkTSSendableFunctionImportedVariables
	arkTSSendableFunctionDecorator
	arkTSSendableTypeAliasDecorator
	arkTSSendableTypeAliasDeclaration
	arkTSSendableFunctionAssignment
	arkTSSendableFunctionOverloadDecorator
	arkTSSendableFunctionProperty
	arkTSSendableFunctionAsExpression
	arkTSSendableDecoratorLimited
	arkTSSharedModuleExportsWarning
	arkTSSendableBetaCompatible
	arkTSSendablePropTypeWarning
	arkTSTaskpoolFunctionArg
	arkTSObjectLiteralAmbiguity
)

type arkTSFaultAttribute struct {
	code     int32
	category diagnostics.Category
	message  string
}

var arkTSFaultAttributes = map[arkTSFault]arkTSFaultAttribute{
	arkTSLiteralAsPropertyName:              {1, diagnostics.CategoryError, "Objects with property names that are not identifiers are not supported (arkts-identifiers-as-prop-names)"},
	arkTSComputedPropertyName:               {1, diagnostics.CategoryError, "Objects with property names that are not identifiers are not supported (arkts-identifiers-as-prop-names)"},
	arkTSSymbolType:                         {2, diagnostics.CategoryError, `"Symbol()" API is not supported (arkts-no-symbol)`},
	arkTSPrivateIdentifier:                  {3, diagnostics.CategoryError, `Private "#" identifiers are not supported (arkts-no-private-identifiers)`},
	arkTSDeclWithDuplicateName:              {4, diagnostics.CategoryError, "Use unique names for types and namespaces. (arkts-unique-names)"},
	arkTSVarDeclaration:                     {5, diagnostics.CategoryError, `Use "let" instead of "var" (arkts-no-var)`},
	arkTSAnyType:                            {8, diagnostics.CategoryError, `Use explicit types instead of "any", "unknown" (arkts-no-any-unknown)`},
	arkTSUnknownType:                        {8, diagnostics.CategoryError, `Use explicit types instead of "any", "unknown" (arkts-no-any-unknown)`},
	arkTSCallSignature:                      {14, diagnostics.CategoryError, `Use "class" instead of a type with call signature (arkts-no-call-signatures)`},
	arkTSConstructorType:                    {15, diagnostics.CategoryError, `Use "class" instead of a type with constructor signature (arkts-no-ctor-signatures-type)`},
	arkTSMultipleStaticBlocks:               {16, diagnostics.CategoryError, "Only one static block is supported (arkts-no-multiple-static-blocks)"},
	arkTSIndexMember:                        {17, diagnostics.CategoryError, "Indexed signatures are not supported (arkts-no-indexed-signatures)"},
	arkTSIntersectionType:                   {19, diagnostics.CategoryError, "Use inheritance instead of intersection types (arkts-no-intersection-types)"},
	arkTSThisType:                           {21, diagnostics.CategoryError, `Type notation using "this" is not supported (arkts-no-typing-with-this)`},
	arkTSConditionalType:                    {22, diagnostics.CategoryError, "Conditional types are not supported (arkts-no-conditional-types)"},
	arkTSParameterProperties:                {25, diagnostics.CategoryError, `Declaring fields in "constructor" is not supported (arkts-no-ctor-prop-decls)`},
	arkTSConstructorIface:                   {27, diagnostics.CategoryError, "Construct signatures are not supported in interfaces (arkts-no-ctor-signatures-iface)"},
	arkTSIndexedAccessType:                  {28, diagnostics.CategoryError, "Indexed access types are not supported (arkts-no-aliases-by-index)"},
	arkTSPropertyAccessByIndex:              {29, diagnostics.CategoryError, "Indexed access is not supported for fields (arkts-no-props-by-index)"},
	arkTSStructuralIdentity:                 {30, diagnostics.CategoryError, "Structural typing is not supported (arkts-no-structural-typing)"},
	arkTSGenericCallNoTypeArgs:              {34, diagnostics.CategoryError, "Type inference in case of generic function calls is limited (arkts-no-inferred-generic-params)"},
	arkTSObjectLiteralNoContextType:         {38, diagnostics.CategoryError, "Object literal must correspond to some explicitly declared class or interface (arkts-no-untyped-obj-literals)"},
	arkTSObjectTypeLiteral:                  {40, diagnostics.CategoryError, "Object literals cannot be used as type declarations (arkts-no-obj-literals-as-types)"},
	arkTSArrayLiteralNoContextType:          {43, diagnostics.CategoryError, "Array literals must contain elements of only inferrable types (arkts-no-noninferrable-arr-literals)"},
	arkTSFunctionExpression:                 {46, diagnostics.CategoryError, "Use arrow functions instead of function expressions (arkts-no-func-expressions)"},
	arkTSClassExpression:                    {50, diagnostics.CategoryError, "Class literals are not supported (arkts-no-class-literals)"},
	arkTSImplementsClass:                    {51, diagnostics.CategoryError, `Classes cannot be specified in "implements" clause (arkts-implements-only-iface)`},
	arkTSMethodReassignment:                 {52, diagnostics.CategoryError, "Reassigning object methods is not supported (arkts-no-method-reassignment)"},
	arkTSTypeAssertion:                      {53, diagnostics.CategoryError, `Only "as T" syntax is supported for type casts (arkts-as-casts)`},
	arkTSJsxElement:                         {54, diagnostics.CategoryError, "JSX expressions are not supported (arkts-no-jsx)"},
	arkTSUnaryArithmNotNumber:               {55, diagnostics.CategoryError, `Unary operators "+", "-" and "~" work only on numbers (arkts-no-polymorphic-unops)`},
	arkTSDeleteOperator:                     {59, diagnostics.CategoryError, `"delete" operator is not supported (arkts-no-delete)`},
	arkTSTypeQuery:                          {60, diagnostics.CategoryError, `"typeof" operator is allowed only in expression contexts (arkts-no-type-query)`},
	arkTSInstanceofUnsupported:              {65, diagnostics.CategoryError, `"instanceof" operator is partially supported (arkts-instanceof-ref-types)`},
	arkTSInOperator:                         {66, diagnostics.CategoryError, `"in" operator is not supported (arkts-no-in)`},
	arkTSDestructuringAssignment:            {69, diagnostics.CategoryError, "Destructuring assignment is not supported (arkts-no-destruct-assignment)"},
	arkTSCommaOperator:                      {71, diagnostics.CategoryError, `The comma operator "," is supported only in "for" loops (arkts-no-comma-outside-loops)`},
	arkTSDestructuringDeclaration:           {74, diagnostics.CategoryError, "Destructuring variable declarations are not supported (arkts-no-destruct-decls)"},
	arkTSCatchWithUnsupportedType:           {79, diagnostics.CategoryError, "Type annotation in catch clause is not supported (arkts-no-types-in-catch)"},
	arkTSForInStatement:                     {80, diagnostics.CategoryError, `"for .. in" is not supported (arkts-no-for-in)`},
	arkTSMappedType:                         {83, diagnostics.CategoryError, "Mapped type expression is not supported (arkts-no-mapped-types)"},
	arkTSWithStatement:                      {84, diagnostics.CategoryError, `"with" statement is not supported (arkts-no-with)`},
	arkTSThrowStatement:                     {87, diagnostics.CategoryError, `"throw" statements cannot accept values of arbitrary types (arkts-limited-throw)`},
	arkTSLimitedReturnTypeInference:         {90, diagnostics.CategoryError, "Function return type inference is limited (arkts-no-implicit-return-types)"},
	arkTSDestructuringParameter:             {91, diagnostics.CategoryError, "Destructuring parameter declarations are not supported (arkts-no-destruct-params)"},
	arkTSLocalFunction:                      {92, diagnostics.CategoryError, "Nested functions are not supported (arkts-no-nested-funcs)"},
	arkTSFunctionContainsThis:               {93, diagnostics.CategoryError, `Using "this" inside stand-alone functions is not supported (arkts-no-standalone-this)`},
	arkTSGeneratorFunction:                  {94, diagnostics.CategoryError, "Generator functions are not supported (arkts-no-generators)"},
	arkTSYieldExpression:                    {94, diagnostics.CategoryError, "Generator functions are not supported (arkts-no-generators)"},
	arkTSIsOperator:                         {96, diagnostics.CategoryError, `Type guarding is supported with "instanceof" and "as" (arkts-no-is)`},
	arkTSSpreadOperator:                     {99, diagnostics.CategoryError, "It is possible to spread only arrays or classes derived from arrays into the rest parameter or array literals (arkts-no-spread)"},
	arkTSInterfaceExtendDifferentProperties: {102, diagnostics.CategoryError, "Interface can not extend interfaces with the same method (arkts-no-extend-same-prop)"},
	arkTSInterfaceMerging:                   {103, diagnostics.CategoryError, "Declaration merging is not supported (arkts-no-decl-merging)"},
	arkTSInterfaceExtendsClass:              {104, diagnostics.CategoryError, "Interfaces cannot extend classes (arkts-extends-only-class)"},
	arkTSConstructorFuncs:                   {106, diagnostics.CategoryError, "Constructor function type is not supported (arkts-no-ctor-signatures-funcs)"},
	arkTSEnumMemberNonConstInit:             {111, diagnostics.CategoryError, "Enumeration members can be initialized only with compile time expressions of the same type (arkts-no-enum-mixed-types)"},
	arkTSEnumMerging:                        {113, diagnostics.CategoryError, `"enum" declaration merging is not supported (arkts-no-enum-merging)`},
	arkTSNamespaceAsObject:                  {114, diagnostics.CategoryError, "Namespaces cannot be used as objects (arkts-no-ns-as-obj)"},
	arkTSNonDeclarationInNamespace:          {116, diagnostics.CategoryError, "Non-declaration statements in namespaces are not supported (single semicolons are considered as empty non-delcaration statements) (arkts-no-ns-statements)"},
	arkTSImportAssignment:                   {121, diagnostics.CategoryError, `"require" and "import" assignment are not supported (arkts-no-require)`},
	arkTSExportAssignment:                   {126, diagnostics.CategoryError, `"export = ..." assignment is not supported (arkts-no-export-assignment)`},
	arkTSShorthandAmbientModuleDecl:         {128, diagnostics.CategoryError, "Ambient module declaration is not supported (arkts-no-ambient-decls)"},
	arkTSWildcardsInModuleName:              {129, diagnostics.CategoryError, "Wildcards in module names are not supported (arkts-no-module-wildcards)"},
	arkTSUMDModuleDefinition:                {130, diagnostics.CategoryError, "Universal module definitions (UMD) are not supported (arkts-no-umd)"},
	arkTSNewTarget:                          {132, diagnostics.CategoryError, `"new.target" is not supported (arkts-no-new-target)`},
	arkTSDefiniteAssignment:                 {134, diagnostics.CategoryWarning, "Definite assignment assertions are not supported (arkts-no-definite-assignment)"},
	arkTSPrototype:                          {136, diagnostics.CategoryError, "Prototype assignment is not supported (arkts-no-prototype-assignment)"},
	arkTSGlobalThis:                         {137, diagnostics.CategoryWarning, `"globalThis" is not supported (arkts-no-globalthis)`},
	arkTSUtilityType:                        {138, diagnostics.CategoryError, "Some of utility types are not supported (arkts-no-utility-types)"},
	arkTSPropertyDeclOnFunction:             {139, diagnostics.CategoryError, "Declaring properties on functions is not supported (arkts-no-func-props)"},
	arkTSFunctionBind:                       {140, diagnostics.CategoryWarning, "'Function.bind' is not supported (arkts-no-func-bind)"},
	arkTSConstAssertion:                     {142, diagnostics.CategoryError, `"as const" assertions are not supported (arkts-no-as-const)`},
	arkTSImportAssertion:                    {143, diagnostics.CategoryError, "Import assertions are not supported (arkts-no-import-assertions)"},
	arkTSLimitedStdLibAPI:                   {144, diagnostics.CategoryError, "Usage of standard library is restricted (arkts-limited-stdlib)"},
	arkTSErrorSuppression:                   {146, diagnostics.CategoryError, "Switching off type checks with in-place comments is not allowed (arkts-strict-typing-required)"},
	arkTSClassAsObject:                      {149, diagnostics.CategoryWarning, "Classes cannot be used as objects (arkts-no-classes-as-obj)"},
	arkTSImportAfterStatement:               {150, diagnostics.CategoryError, `"import" statements after other statements are not allowed (arkts-no-misplaced-imports)`},
	arkTSESObjectType:                       {151, diagnostics.CategoryWarning, `Usage of 'ESObject' type is restricted (arkts-limited-esobj)`},
	arkTSFunctionApplyCall:                  {152, diagnostics.CategoryError, "'Function.apply', 'Function.call' are not supported (arkts-no-func-apply-call)"},
	arkTSSendableClassInheritance:           {153, diagnostics.CategoryError, `The inheritance for "Sendable" classes is limited (arkts-sendable-class-inheritance)`},
	arkTSSendablePropType:                   {154, diagnostics.CategoryError, `Properties in "Sendable" classes and interfaces must have a Sendable data type (arkts-sendable-prop-types)`},
	arkTSSendableDefiniteAssignment:         {155, diagnostics.CategoryError, `Definite assignment assertion is not allowed in "Sendable" classes (arkts-sendable-definite-assignment)`},
	arkTSSendableGenericTypes:               {156, diagnostics.CategoryError, `Type arguments of generic "Sendable" type must be a "Sendable" data type (arkts-sendable-generic-types)`},
	arkTSSendableCapturedVars:               {157, diagnostics.CategoryError, `Only imported variables can be captured by "Sendable" class (arkts-sendable-imported-variables)`},
	arkTSSendableClassDecorator:             {158, diagnostics.CategoryError, `Only "@Sendable" decorator can be used on "Sendable" class (arkts-sendable-class-decorator)`},
	arkTSSendableObjectInitialization:       {159, diagnostics.CategoryError, `Objects of "Sendable" type can not be initialized using object literal or array literal (arkts-sendable-obj-init)`},
	arkTSSendableComputedPropName:           {160, diagnostics.CategoryError, `Computed property names are not allowed in "Sendable" classes and interfaces (arkts-sendable-computed-prop-name)`},
	arkTSSendableAsExpression:               {161, diagnostics.CategoryError, `Casting "Non-sendable" data to "Sendable" type is not allowed (arkts-sendable-as-expr)`},
	arkTSSharedNoSideEffectImport:           {162, diagnostics.CategoryError, "Importing a module for side-effects only is not supported in shared module (arkts-no-side-effects-imports)"},
	arkTSSharedModuleExports:                {163, diagnostics.CategoryError, `Only "Sendable" entities can be exported in shared module (arkts-shared-module-exports)`},
	arkTSSharedModuleNoWildcardExport:       {164, diagnostics.CategoryError, `"export * from ..." is not allowed in shared module (arkts-shared-module-no-wildcard-export)`},
	arkTSNoTSImportETS:                      {165, diagnostics.CategoryError, `Only "Sendable" classes and "Sendable" interfaces are allowed for importing from ets into ts file (arkts-no-ts-import-ets)`},
	arkTSSendableTypeInheritance:            {166, diagnostics.CategoryError, `In ts files, "Sendable" types cannot be used in implements and extends clauses (arkts-no-ts-sendable-type-inheritance)`},
	arkTSSendableTypeExported:               {167, diagnostics.CategoryError, `In sdk ts files, "Sendable" class and "Sendable" interface can not be exported (arkts-no-dts-sendable-type-export)`},
	arkTSNoTSReExportETS:                    {168, diagnostics.CategoryError, "In ts files, entities from ets files can not be re-exported (arkts-no-ts-re-export-ets)"},
	arkTSNoNamespaceImportETSToTS:           {169, diagnostics.CategoryError, "Namespace import is not allowed for importing from ets to ts file (arkts-no-namespace-import-in-ts-import-ets)"},
	arkTSNoSideEffectImportETSToTS:          {170, diagnostics.CategoryError, "Side effect import is not allowed for importing from ets to ts file (artkts-no-side-effect-import-in-ts-import-ets)"},
	arkTSSendableExplicitFieldType:          {171, diagnostics.CategoryError, "Field in sendable class must have type annotation (arkts-sendable-explicit-field-type)"},
	arkTSSendableFunctionImportedVariables:  {172, diagnostics.CategoryError, `Only imported variables can be captured by "Sendable" function (arkts-sendable-function-imported-variables)`},
	arkTSSendableFunctionDecorator:          {173, diagnostics.CategoryError, `Only "@Sendable" decorator can be used on "Sendable" function (arkts-sendable-function-decorator)`},
	arkTSSendableTypeAliasDecorator:         {174, diagnostics.CategoryError, `Only "@Sendable" decorator can be used on "Sendable" typeAlias (arkts-sendable-typealias-decorator)`},
	arkTSSendableTypeAliasDeclaration:       {175, diagnostics.CategoryError, `Only "FunctionType" can declare "Sendable" typeAlias (arkts-sendable-typeAlias-declaration)`},
	arkTSSendableFunctionAssignment:         {176, diagnostics.CategoryError, `Only "Sendable" function or "Sendable" typeAlias object can be assigned to "Sendable" typeAlias (arkts-sendable-function-assignment)`},
	arkTSSendableFunctionOverloadDecorator:  {177, diagnostics.CategoryError, `When declaring "@Sendable" overloaded function, needs to add "@Sendable" decorator on each function (arkts-sendable-function-overload-decorator)`},
	arkTSSendableFunctionProperty:           {178, diagnostics.CategoryError, `The property of "Sendable" function is limited (arkts-sendable-function-property)`},
	arkTSSendableFunctionAsExpression:       {179, diagnostics.CategoryError, `Casting "Non-sendable" function to "Sendable" typeAlias is not allowed (arkts-sendable-function-as-expr)`},
	arkTSSendableDecoratorLimited:           {180, diagnostics.CategoryError, `The "@Sendable" decorator can only be used on "class", "function" and "typeAlias" (arkts-sendable-decorator-limited)`},
	arkTSSharedModuleExportsWarning:         {163, diagnostics.CategoryWarning, `Only "Sendable" entities can be exported in shared module (arkts-shared-module-exports)`},
	arkTSSendableBetaCompatible:             {182, diagnostics.CategoryError, "Sendable functions and sendable typealias are not available when compatibleSdkVersionStage is lowering than beta3 of API12 (arkts-sendable-beta-compatible)"},
	arkTSSendablePropTypeWarning:            {154, diagnostics.CategoryWarning, `Properties in "Sendable" classes and interfaces must have a Sendable data type (arkts-sendable-prop-types)`},
	arkTSTaskpoolFunctionArg:                {183, diagnostics.CategoryError, `The function passed to taskpool must be an ordinary function decorated with "@Concurrent" (arkts-taskpool-concurrent-function-args)`},
	arkTSObjectLiteralAmbiguity:             {184, diagnostics.CategoryError, "The object literal is ambiguity, the type of 1.2 cannot exist (arkts-no-ambiguity-obj-literal)"},
}

type arkTSLinter struct {
	checker           *Checker
	file              *ast.SourceFile
	strictDiagnostics []*ast.Diagnostic
	diagnostics       []*ast.Diagnostic
	exported          map[*ast.Node]struct{}
	kitInfos          map[string]arkTSKitInfo
}

type arkTSKitInfo struct {
	Symbols map[string]arkTSKitSymbol `json:"symbols"`
}

type arkTSKitSymbol struct {
	Source string `json:"source"`
}

type arkTSStrictErrorType uint8

const (
	arkTSStrictNoError arkTSStrictErrorType = iota
	arkTSStrictUnknown
	arkTSStrictNull
	arkTSStrictPossiblyUndefined
)

func (c *Checker) GetArkTSLinterDiagnostics(file *ast.SourceFile, strictDiagnostics []*ast.Diagnostic) []*ast.Diagnostic {
	var nonLinterDiagnostics []*ast.Diagnostic
	if c.compilerOptions.StrictCheckerOnly.IsTrue() && file.ScriptKind == core.ScriptKindETS {
		linterDiagnostics := make([]*ast.Diagnostic, 0, len(strictDiagnostics))
		nonLinterDiagnostics = make([]*ast.Diagnostic, 0, len(strictDiagnostics))
		for _, diagnostic := range strictDiagnostics {
			switch diagnostic.Code() {
			case 2322, 2345, 2769, 2532, 2564:
				linterDiagnostics = append(linterDiagnostics, diagnostic)
			default:
				nonLinterDiagnostics = append(nonLinterDiagnostics, diagnostic)
			}
		}
		strictDiagnostics = linterDiagnostics
	}
	linter := arkTSLinter{checker: c, file: file, strictDiagnostics: slices.Clone(strictDiagnostics)}
	if file.ScriptKind == core.ScriptKindETS {
		linter.visit(file.AsNode())
		linter.checkCommentDirectives()
	} else {
		linter.visitInterop(file.AsNode())
	}
	result := append(linter.strictDiagnostics, nonLinterDiagnostics...)
	return append(result, linter.diagnostics...)
}

func (l *arkTSLinter) add(node *ast.Node, fault arkTSFault) {
	attribute, ok := arkTSFaultAttributes[fault]
	if !ok || node == nil {
		return
	}
	loc := core.NewTextRange(scanner.GetTokenPosOfNode(node, l.file, false), node.End())
	switch fault {
	case arkTSVarDeclaration:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+3, loc.End()))
	case arkTSDeleteOperator:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+6, loc.End()))
	case arkTSTypeQuery:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+6, loc.End()))
	case arkTSWithStatement:
		if statement := node.AsWithStatement().Statement; statement != nil {
			loc = core.NewTextRange(loc.Pos(), scanner.GetErrorRangeForNode(l.file, statement).Pos()-1)
		}
	case arkTSForInStatement:
		statement := node.AsForInOrOfStatement()
		loc = core.NewTextRange(statement.Initializer.End()+1, statement.Expression.Pos()-1)
	case arkTSCatchWithUnsupportedType:
		if declaration := node.AsCatchClause().VariableDeclaration; declaration != nil {
			loc = scanner.GetErrorRangeForNode(l.file, declaration)
		}
	case arkTSDeclWithDuplicateName:
		if name := node.Name(); name != nil {
			loc = scanner.GetErrorRangeForNode(l.file, name)
		}
	case arkTSObjectLiteralNoContextType:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+1, loc.End()))
	case arkTSInstanceofUnsupported:
		operator := node.AsBinaryExpression().OperatorToken
		start := scanner.GetErrorRangeForNode(l.file, operator).Pos()
		loc = core.NewTextRange(start, start+len("instanceof"))
	case arkTSConstAssertion:
		if ast.IsAsExpression(node) {
			assertion := node.AsAsExpression()
			loc = core.NewTextRange(assertion.Expression.End()+1, scanner.GetErrorRangeForNode(l.file, assertion.Type).Pos()-1)
		} else if node.Kind == ast.KindTypeAssertionExpression {
			assertion := node.AsTypeAssertion()
			loc = core.NewTextRange(assertion.Expression.End()+1, assertion.Type.End()+1)
		}
	case arkTSLimitedReturnTypeInference:
		var target *ast.Node
		switch node.Kind {
		case ast.KindFunctionExpression:
			target = node.Type()
		case ast.KindFunctionDeclaration, ast.KindMethodDeclaration:
			target = node.Name()
		}
		if target != nil {
			loc = scanner.GetErrorRangeForNode(l.file, target)
		}
	case arkTSLocalFunction:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+len("function"), loc.End()))
	case arkTSFunctionApplyCall, arkTSFunctionBind:
		text := l.file.Text()[loc.Pos():loc.End()]
		if point := strings.LastIndex(text, "."); point >= 0 {
			loc = core.NewTextRange(loc.Pos()+point+1, loc.End())
		}
	case arkTSClassExpression:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+5, loc.End()))
	case arkTSMultipleStaticBlocks:
		loc = core.NewTextRange(loc.Pos(), min(loc.Pos()+6, loc.End()))
	case arkTSSendableDefiniteAssignment:
		name := node.Name()
		if name != nil {
			end := name.End()
			if token := node.AsPropertyDeclaration().PostfixToken; token != nil && token.Kind == ast.KindExclamationToken {
				end = token.End()
			}
			loc = core.NewTextRange(scanner.GetErrorRangeForNode(l.file, name).Pos(), end)
		}
	}
	l.diagnostics = append(l.diagnostics, ast.NewDiagnosticFromText(
		l.file,
		loc,
		attribute.code,
		attribute.category,
		attribute.message,
		nil,
		nil,
		false,
		false,
	))
}

func (l *arkTSLinter) visit(node *ast.Node) {
	if node == nil {
		return
	}
	if node.Parent != nil && ast.IsStructDeclaration(node.Parent) && ast.IsConstructorDeclaration(node) && node.Virtual {
		return
	}
	if fault, ok := arkTSIncrementOnly[node.Kind]; ok {
		l.add(node, fault)
	} else {
		l.handle(node)
	}
	node.ForEachChild(func(child *ast.Node) bool {
		l.visit(child)
		return false
	})
}

var arkTSIncrementOnly = map[ast.Kind]arkTSFault{
	ast.KindAnyKeyword:                 arkTSAnyType,
	ast.KindSymbolKeyword:              arkTSSymbolType,
	ast.KindThisType:                   arkTSThisType,
	ast.KindTypeQuery:                  arkTSTypeQuery,
	ast.KindDeleteExpression:           arkTSDeleteOperator,
	ast.KindTypePredicate:              arkTSIsOperator,
	ast.KindYieldExpression:            arkTSYieldExpression,
	ast.KindWithStatement:              arkTSWithStatement,
	ast.KindIndexedAccessType:          arkTSIndexedAccessType,
	ast.KindUnknownKeyword:             arkTSUnknownType,
	ast.KindCallSignature:              arkTSCallSignature,
	ast.KindIntersectionType:           arkTSIntersectionType,
	ast.KindTypeLiteral:                arkTSObjectTypeLiteral,
	ast.KindConstructorType:            arkTSConstructorFuncs,
	ast.KindPrivateIdentifier:          arkTSPrivateIdentifier,
	ast.KindConditionalType:            arkTSConditionalType,
	ast.KindMappedType:                 arkTSMappedType,
	ast.KindJsxElement:                 arkTSJsxElement,
	ast.KindJsxSelfClosingElement:      arkTSJsxElement,
	ast.KindImportEqualsDeclaration:    arkTSImportAssignment,
	ast.KindNamespaceExportDeclaration: arkTSUMDModuleDefinition,
	ast.KindClassExpression:            arkTSClassExpression,
}

func (l *arkTSLinter) handle(node *ast.Node) {
	switch node.Kind {
	case ast.KindObjectLiteralExpression:
		l.handleObjectLiteralExpression(node)
	case ast.KindArrayLiteralExpression:
		l.handleArrayLiteralExpression(node)
	case ast.KindParameter:
		l.handleParameter(node)
	case ast.KindForStatement:
		if initializer := node.AsForStatement().Initializer; initializer != nil && (ast.IsArrayLiteralExpression(initializer) || ast.IsObjectLiteralExpression(initializer)) {
			l.add(initializer, arkTSDestructuringAssignment)
		}
	case ast.KindForInStatement:
		initializer := node.AsForInOrOfStatement().Initializer
		if ast.IsArrayLiteralExpression(initializer) || ast.IsObjectLiteralExpression(initializer) {
			l.add(initializer, arkTSDestructuringAssignment)
		}
		l.add(node, arkTSForInStatement)
	case ast.KindForOfStatement:
		initializer := node.AsForInOrOfStatement().Initializer
		if ast.IsArrayLiteralExpression(initializer) || ast.IsObjectLiteralExpression(initializer) {
			l.add(initializer, arkTSDestructuringAssignment)
		}
	case ast.KindImportDeclaration:
		l.handleImportDeclaration(node)
	case ast.KindImportClause, ast.KindImportSpecifier, ast.KindNamespaceImport:
		if node.Name() != nil {
			l.countDuplicateName(node.Name(), node, node.Kind)
		}
	case ast.KindPropertyDeclaration, ast.KindPropertySignature:
		if name := node.Name(); ast.IsNumericLiteral(name) {
			l.add(name, arkTSLiteralAsPropertyName)
		}
		if ast.IsPropertyDeclaration(node) {
			l.filterPropertyInitializationDiagnostic(node)
		}
		l.handleInferredDeclaration(node)
		l.handleDefiniteAssignment(node)
		if ast.IsPropertyDeclaration(node) {
			l.handlePropertyAssignmentCompatibility(node)
			l.handleSendableClassProperty(node)
		} else {
			l.handleSendableInterfaceProperty(node)
		}
	case ast.KindPropertyAssignment:
		if name := node.Name(); ast.IsNumericLiteral(name) && !l.numericPropertyAssignmentIsAllowed(node) {
			l.add(name, arkTSLiteralAsPropertyName)
		}
	case ast.KindFunctionExpression:
		l.handleFunctionExpression(node)
	case ast.KindArrowFunction:
		l.handleStandaloneThis(node, node.Body())
		l.handleMissingReturnType(node)
	case ast.KindFunctionDeclaration:
		l.handleFunctionDeclaration(node)
	case ast.KindTypeAliasDeclaration:
		if node.Name() != nil {
			l.countDuplicateName(node.Name(), node, node.Kind)
		}
		l.handleSendableTypeAlias(node)
	case ast.KindPrefixUnaryExpression:
		l.handlePrefixUnaryExpression(node)
	case ast.KindBinaryExpression:
		l.handleBinaryExpression(node)
	case ast.KindVariableDeclarationList:
		if node.Flags&(ast.NodeFlagsLet|ast.NodeFlagsConst) == 0 {
			l.add(node, arkTSVarDeclaration)
		}
	case ast.KindVariableDeclaration:
		l.handleVariableDeclaration(node)
	case ast.KindCatchClause:
		if declaration := node.AsCatchClause().VariableDeclaration; declaration != nil && declaration.Type() != nil {
			l.add(node, arkTSCatchWithUnsupportedType)
		}
	case ast.KindClassDeclaration:
		l.handleClassDeclaration(node)
	case ast.KindInterfaceDeclaration:
		l.handleInterfaceDeclaration(node)
	case ast.KindEnumDeclaration:
		l.handleEnumDeclaration(node)
	case ast.KindModuleDeclaration:
		l.handleModuleDeclaration(node)
	case ast.KindTypeAssertionExpression:
		start := scanner.GetTokenPosOfNode(node, l.file, false)
		// OH's ETS parser does not materialize a TypeAssertionExpression for a
		// JSX closing tag. The Go frontend represents that recovery node as an
		// assertion, so exclude only the parser-shape mismatch before applying
		// TypeScriptLinter.ts::handleTypeAssertionExpression.
		if strings.HasPrefix(l.file.Text()[start:node.End()], "</") {
			lineStart := strings.LastIndex(l.file.Text()[:start], "\n") + 1
			if strings.TrimSpace(l.file.Text()[lineStart:start]) == "" {
				break
			}
		}
		if ast.IsConstTypeReference(node.AsTypeAssertion().Type) {
			l.add(node, arkTSConstAssertion)
		} else {
			l.add(node, arkTSTypeAssertion)
		}
	case ast.KindMethodDeclaration, ast.KindMethodSignature:
		if ast.IsMethodDeclaration(node) {
			l.handleDecoratorsInSendableClass(node)
			l.filterMethodReturnDiagnostic(node)
		}
		l.handleMissingReturnType(node)
		if ast.IsMethodDeclaration(node) && node.ModifierFlags()&ast.ModifierFlagsStatic != 0 {
			l.reportThisKeywords(node.Body())
		}
		if arkTSAsteriskToken(node) != nil {
			l.add(node, arkTSGeneratorFunction)
		}
	case ast.KindIdentifier:
		l.handleIdentifier(node)
	case ast.KindPropertyAccessExpression:
		l.handlePropertyAccess(node)
	case ast.KindElementAccessExpression:
		l.handleElementAccess(node)
	case ast.KindEnumMember:
		l.handleEnumMember(node)
	case ast.KindTypeReference:
		l.handleTypeReference(node)
	case ast.KindExportAssignment:
		if node.AsExportAssignment().IsExportEquals {
			l.add(node, arkTSExportAssignment)
		}
		l.handleSharedExportAssignment(node)
	case ast.KindExportKeyword:
		l.handleSharedExportKeyword(node)
	case ast.KindExportDeclaration:
		l.handleSharedExportDeclaration(node)
	case ast.KindCallExpression:
		l.handleCallExpression(node)
	case ast.KindNewExpression:
		l.handleGenericCall(node)
	case ast.KindMetaProperty:
		if node.AsMetaProperty().KeywordToken == ast.KindNewKeyword && node.Name().Text() == "target" {
			l.add(node, arkTSNewTarget)
		}
	case ast.KindAsExpression:
		assertion := node.AsAsExpression()
		if ast.IsConstTypeReference(assertion.Type) {
			l.add(node, arkTSConstAssertion)
		}
		targetType := l.checker.GetNonNullableType(l.checker.getTypeOfNode(assertion.Type))
		expressionType := l.checker.GetNonNullableType(l.checker.getTypeOfNode(assertion.Expression))
		if expressionType != nil && targetType != nil &&
			(expressionType.flags&TypeFlagsNumberLike != 0 && targetType == l.checker.globalNumberType ||
				expressionType.flags&TypeFlagsBooleanLike != 0 && targetType == l.checker.globalBooleanType) {
			l.add(node, arkTSTypeAssertion)
		}
		l.handleSendableAsExpression(node, targetType, expressionType)
	case ast.KindSpreadElement, ast.KindSpreadAssignment:
		l.handleSpread(node)
	case ast.KindConstructSignature:
		if ast.IsTypeLiteralNode(node.Parent) {
			l.add(node, arkTSConstructorType)
		} else if ast.IsInterfaceDeclaration(node.Parent) {
			l.add(node, arkTSConstructorIface)
		}
	case ast.KindComputedPropertyName:
		l.handleComputedPropertyName(node)
	case ast.KindClassStaticBlockDeclaration:
		if l.checker.compilerOptions.SkipArkTSStaticBlocksCheck != core.TSTrue {
			l.reportThisKeywords(node.AsClassStaticBlockDeclaration().Body)
		}
	case ast.KindIndexSignature:
		if !l.isAllowedIndexSignature(node) {
			l.add(node, arkTSIndexMember)
		}
	case ast.KindThrowStatement:
		l.handleThrow(node)
	case ast.KindReturnStatement:
		l.handleReturnStatement(node)
	case ast.KindExpressionWithTypeArguments:
		l.handleExpressionWithTypeArguments(node)
	case ast.KindDecorator:
		l.handleDecorator(node)
	case ast.KindGetAccessor, ast.KindSetAccessor:
		l.handleDecoratorsInSendableClass(node)
	case ast.KindTypeParameter:
		l.handleSendableTypeParameter(node)
	}
}

func (l *arkTSLinter) handleObjectLiteralExpression(node *ast.Node) {
	if l.isDestructuringAssignmentLHS(node) {
		return
	}
	contextualType := l.checker.getContextualType(node, ContextFlagsNone)
	if contextualType != nil && l.isLiteralInitializerForSendableType(contextualType, node) {
		l.add(node, arkTSSendableObjectInitialization)
		return
	}
	if !l.isStructObjectInitializer(node) && !l.isDynamicLiteralInitializer(node) && !l.isObjectLiteralAssignable(contextualType, node) {
		l.add(node, arkTSObjectLiteralNoContextType)
	}
}

func (l *arkTSLinter) handleArrayLiteralExpression(node *ast.Node) {
	if l.isDestructuringAssignmentLHS(node) {
		return
	}
	contextualType := l.checker.getContextualType(node, ContextFlagsNone)
	if contextualType != nil && l.isLiteralInitializerForSendableType(contextualType, node) {
		l.add(node, arkTSSendableObjectInitialization)
		return
	}
	for _, element := range node.AsArrayLiteralExpression().Elements.Nodes {
		if ast.IsOmittedExpression(element) {
			continue
		}
		elementType := l.checker.getContextualType(element, ContextFlagsNone)
		if ast.IsObjectLiteralExpression(element) && !l.isDynamicLiteralInitializer(node) && !l.isObjectLiteralAssignable(elementType, element) {
			l.add(node, arkTSArrayLiteralNoContextType)
			return
		}
		if elementType != nil {
			l.checkAssignmentMatching(element, elementType, element, true)
		}
	}
}

func (l *arkTSLinter) isDestructuringAssignmentLHS(node *ast.Node) bool {
	current := node
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if ast.IsBinaryExpression(parent) {
			binary := parent.AsBinaryExpression()
			if isAssignmentOperator(binary.OperatorToken.Kind) && binary.Left == current {
				return true
			}
		}
		if ast.IsForStatement(parent) && parent.AsForStatement().Initializer == current {
			return true
		}
		if (ast.IsForInStatement(parent) || ast.IsForOfStatement(parent)) && parent.AsForInOrOfStatement().Initializer == current {
			return true
		}
		current = parent
	}
	return false
}

func (l *arkTSLinter) isStructObjectInitializer(node *ast.Node) bool {
	if node.Parent == nil || !ast.IsCallLikeExpression(node.Parent) {
		return false
	}
	signature := l.checker.getResolvedSignature(node.Parent, nil, CheckModeNormal)
	return signature != nil && signature.declaration != nil && ast.IsConstructorDeclaration(signature.declaration) && ast.IsStructDeclaration(signature.declaration.Parent)
}

func (l *arkTSLinter) isObjectLiteralAssignable(contextualType *Type, node *ast.Node) bool {
	if contextualType == nil {
		return false
	}
	contextualType = l.checker.GetNonNullableType(contextualType)
	if contextualType.IsUnion() {
		for _, component := range contextualType.Types() {
			if l.isObjectLiteralAssignable(component, node) {
				return true
			}
		}
		return false
	}
	if (IsTypeAny(contextualType) || l.isLibraryType(contextualType)) && !l.isStaticSourceType(contextualType) {
		return true
	}
	if contextualType.alias != nil && contextualType.alias.Symbol() != nil {
		aliasName := contextualType.alias.Symbol().Name
		if (aliasName == "Partial" || aliasName == "Required" || aliasName == "Readonly") && l.checker.GetFullyQualifiedName(contextualType.alias.Symbol()) == aliasName {
			arguments := contextualType.alias.TypeArguments()
			if len(arguments) != 1 {
				return false
			}
			contextualType = arguments[0]
		}
	}
	if l.isStdRecordType(contextualType) {
		return l.hasValidRecordKeys(node)
	}
	target := contextualType
	if contextualType.flags&TypeFlagsObject != 0 && contextualType.objectFlags&ObjectFlagsReference != 0 && contextualType.AsObjectType().target != nil {
		target = contextualType.AsObjectType().target
	}
	if target.objectFlags&ObjectFlagsClassOrInterface == 0 || !l.hasDefaultConstructor(target) || l.hasReadonlyFields(target) || l.isAbstractClass(target) || l.hasMethods(target) {
		return false
	}
	for _, property := range node.AsObjectLiteralExpression().Properties.Nodes {
		if !ast.IsPropertyAssignment(property) {
			continue
		}
		name := l.propertyName(property.Name())
		propertySymbol := l.checker.getPropertyOfType(target, name)
		if name == "" || propertySymbol == nil || len(propertySymbol.Declarations) == 0 {
			return false
		}
		propertyType := l.checker.GetTypeOfSymbolAtLocation(propertySymbol, propertySymbol.Declarations[0])
		initializer := ast.SkipParentheses(property.Initializer())
		if ast.IsObjectLiteralExpression(initializer) {
			if !l.isObjectLiteralAssignable(propertyType, initializer) {
				return false
			}
			continue
		}
		rightType := l.checker.getTypeOfNode(initializer)
		if l.needsStructuralIdentity(propertyType, rightType, initializer, l.needsStrictStructuralMatch(propertyType, rightType)) || l.isWrongSendableFunctionAssignment(propertyType, rightType) {
			return false
		}
	}
	return true
}

func (l *arkTSLinter) propertyName(name *ast.Node) string {
	if ast.IsIdentifier(name) || ast.IsPrivateIdentifier(name) || ast.IsStringOrNumericLiteralLike(name) {
		return name.Text()
	}
	if ast.IsComputedPropertyName(name) {
		if value := l.checker.evaluate(name.AsComputedPropertyName().Expression, name).Value; value != nil {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func (l *arkTSLinter) hasDefaultConstructor(t *Type) bool {
	if t == nil || t.symbol == nil {
		return true
	}
	hasConstructor := false
	for _, symbol := range t.symbol.Members {
		if symbol.Flags&ast.SymbolFlagsConstructor == 0 {
			continue
		}
		hasConstructor = true
		if len(symbol.Declarations) > 0 && ast.IsConstructorDeclaration(symbol.Declarations[0]) && len(symbol.Declarations[0].Parameters()) == 0 {
			return true
		}
	}
	return !hasConstructor
}

func (l *arkTSLinter) hasReadonlyFields(t *Type) bool {
	if t == nil || t.symbol == nil {
		return false
	}
	for _, symbol := range t.symbol.Members {
		if len(symbol.Declarations) > 0 && ast.IsPropertyDeclaration(symbol.Declarations[0]) && ast.HasModifier(symbol.Declarations[0], ast.ModifierFlagsReadonly) {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) isAbstractClass(t *Type) bool {
	return t != nil && t.IsClass() && t.symbol != nil && len(t.symbol.Declarations) > 0 && ast.HasModifier(t.symbol.Declarations[0], ast.ModifierFlagsAbstract)
}

func (l *arkTSLinter) hasMethods(t *Type) bool {
	for _, property := range l.checker.getPropertiesOfType(t) {
		if property.Flags&ast.SymbolFlagsMethod != 0 {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) isStdRecordType(t *Type) bool {
	if t == nil || t.alias == nil || t.alias.Symbol() == nil || t.alias.Symbol().Name != "Record" {
		return false
	}
	return l.checker.GetFullyQualifiedName(t.alias.Symbol()) == "Record"
}

func (l *arkTSLinter) hasValidRecordKeys(node *ast.Node) bool {
	for _, property := range node.AsObjectLiteralExpression().Properties.Nodes {
		name := property.Name()
		if name == nil || !(ast.IsStringLiteral(name) || ast.IsNumericLiteral(name) || ast.IsComputedPropertyName(name) && l.isValidComputedPropertyName(name, true)) {
			return false
		}
	}
	return true
}

func (l *arkTSLinter) numericPropertyAssignmentIsAllowed(node *ast.Node) bool {
	contextualType := l.checker.getContextualType(node.Parent, ContextFlagsNone)
	return contextualType != nil && (l.isStdRecordType(contextualType) || l.isLibraryType(contextualType)) || l.isDynamicLiteralInitializer(node.Parent)
}

func (l *arkTSLinter) handleParameter(node *ast.Node) {
	l.handleDecoratorsInSendableClass(node)
	name := node.Name()
	if ast.IsArrayBindingPattern(name) || ast.IsObjectBindingPattern(name) {
		l.add(node, arkTSDestructuringParameter)
	}
	for _, modifier := range node.ModifierNodes() {
		switch modifier.Kind {
		case ast.KindPublicKeyword, ast.KindProtectedKeyword, ast.KindReadonlyKeyword, ast.KindPrivateKeyword:
			l.add(node, arkTSParameterProperties)
			return
		}
	}
	l.handleInferredDeclaration(node)
}

func (l *arkTSLinter) decorators(node *ast.Node) []*ast.Node {
	var result []*ast.Node
	for _, modifier := range node.ModifierNodes() {
		if ast.IsDecorator(modifier) {
			result = append(result, modifier)
		}
	}
	return result
}

func (l *arkTSLinter) reportNonSendableDecorators(node *ast.Node, fault arkTSFault) {
	disableClassRule := fault == arkTSSendableClassDecorator
	if disableClassRule {
		disableClassRule = false
		for _, rule := range l.checker.compilerOptions.DisableSendableCheckRules {
			if rule == "arkts-sendable-class-decorator" {
				disableClassRule = true
				break
			}
		}
	}
	for _, decorator := range l.decorators(node) {
		if l.decoratorName(decorator) == "Sendable" {
			continue
		}
		if disableClassRule && !arkTSIsArkUIDecorator(l.decoratorName(decorator)) {
			continue
		}
		l.add(decorator, fault)
	}
}

func arkTSIsArkUIDecorator(name string) bool {
	switch name {
	case "Entry", "Component", "Reusable", "CustomDialog", "Consume", "Link", "LocalStorageLink", "LocalStorageProp",
		"ObjectLink", "Prop", "Provide", "State", "StorageLink", "StorageProp", "Builder", "LocalBuilder",
		"BuilderParam", "Observed", "Require", "Sendable", "Track", "ComponentV2", "ObservedV2", "Trace",
		"Local", "Param", "Once", "Event", "Monitor", "Provider", "Consumer", "Computed", "Type", "Env":
		return true
	default:
		return false
	}
}

func (l *arkTSLinter) filterPropertyInitializationDiagnostic(node *ast.Node) {
	name := node.Name()
	if name == nil {
		return
	}
	propertyDecorator := false
	for _, decorator := range l.decorators(node) {
		switch l.decoratorName(decorator) {
		case "Link", "Consume", "ObjectLink", "Prop", "BuilderParam", "Param", "Event", "Require", "Env":
			propertyDecorator = true
		}
	}
	classDecorator := false
	if node.Type() != nil && l.nodeText(node.Type()) == "CustomDialogController" && node.Parent != nil {
		for _, decorator := range l.decorators(node.Parent) {
			if l.decoratorName(decorator) == "CustomDialog" {
				classDecorator = true
				break
			}
		}
	}
	if propertyDecorator || classDecorator {
		start := scanner.GetTokenPosOfNode(name, l.file, false)
		l.filterStrictDiagnostics(2564, start, start)
	}
}

func (l *arkTSLinter) filterMethodReturnDiagnostic(node *ast.Node) {
	found := false
	for _, decorator := range l.decorators(node) {
		switch l.decoratorName(decorator) {
		case "AnimatableExtend", "Builder", "Extend", "Styles":
			found = true
		}
	}
	if !found {
		return
	}
	parameters := node.ParameterList()
	begin := parameters.End()
	end := begin
	if node.Body() != nil {
		end = scanner.GetTokenPosOfNode(node.Body(), l.file, false)
	}
	l.filterStrictDiagnostics(2366, begin, end)
}

func (l *arkTSLinter) filterStrictDiagnostics(code int32, begin int, end int) {
	l.strictDiagnostics = slices.DeleteFunc(l.strictDiagnostics, func(diagnostic *ast.Diagnostic) bool {
		return diagnostic.Code() == code && diagnostic.Pos() >= begin && diagnostic.Pos() <= end
	})
}

func (l *arkTSLinter) nodeText(node *ast.Node) string {
	if node == nil {
		return ""
	}
	start := scanner.GetTokenPosOfNode(node, l.file, false)
	if start < 0 || start > node.End() || node.End() > len(l.file.Text()) {
		return ""
	}
	return l.file.Text()[start:node.End()]
}

func (l *arkTSLinter) handleDecoratorsInSendableClass(declaration *ast.Node) {
	var class *ast.Node
	if ast.IsParameterDeclaration(declaration) && declaration.Parent != nil {
		class = declaration.Parent.Parent
	} else {
		class = declaration.Parent
	}
	if class == nil || !ast.IsClassDeclaration(class) || ast.IsStructDeclaration(class) || !l.hasDecorator(class, "Sendable") {
		return
	}
	for _, decorator := range l.decorators(declaration) {
		l.add(decorator, arkTSSendableClassDecorator)
	}
}

func (l *arkTSLinter) handleSendableClassProperty(node *ast.Node) {
	class := node.Parent
	if class == nil || !ast.IsClassDeclaration(class) || ast.IsStructDeclaration(class) || !l.hasDecorator(class, "Sendable") {
		return
	}
	if node.Type() == nil {
		l.add(node, arkTSSendableExplicitFieldType)
		return
	}
	l.handleDecoratorsInSendableClass(node)
	if !l.isSendableTypeNode(node.Type(), false, make(map[*ast.Symbol]struct{})) {
		l.add(node, arkTSSendablePropType)
	} else {
		l.checkTypeAliasInSendableScope(node)
	}
}

func (l *arkTSLinter) handleSendableInterfaceProperty(node *ast.Node) {
	if node.Type() == nil || node.Parent == nil || !ast.IsInterfaceDeclaration(node.Parent) || !l.isSendableClassOrInterface(l.checker.getTypeOfNode(node.Parent)) {
		return
	}
	if !l.isSendableTypeNode(node.Type(), false, make(map[*ast.Symbol]struct{})) {
		l.add(node, arkTSSendablePropType)
	} else {
		l.checkTypeAliasInSendableScope(node)
	}
}

func (l *arkTSLinter) checkTypeAliasInSendableScope(node *ast.Node) {
	typeNode := node.Type()
	for ast.IsParenthesizedTypeNode(typeNode) {
		typeNode = typeNode.AsParenthesizedTypeNode().Type
	}
	if ast.IsUnionTypeNode(typeNode) {
		for _, component := range typeNode.AsUnionTypeNode().Types.Nodes {
			if l.isNonSendableTypeAliasReference(component) {
				l.add(typeNode, arkTSSendablePropTypeWarning)
				return
			}
		}
		return
	}
	if l.isNonSendableTypeAliasReference(typeNode) {
		l.add(typeNode, arkTSSendablePropTypeWarning)
	}
}

func (l *arkTSLinter) isNonSendableTypeAliasReference(node *ast.Node) bool {
	if !ast.IsTypeReferenceNode(node) {
		return false
	}
	reference := node.AsTypeReferenceNode()
	symbol := l.trueSymbolAtLocation(reference.TypeName)
	if symbol == nil || symbol.Flags&ast.SymbolFlagsTypeAlias == 0 || len(symbol.Declarations) == 0 || !ast.IsTypeAliasDeclaration(symbol.Declarations[0]) {
		return false
	}
	if reference.TypeArguments == nil {
		return false
	}
	for _, argument := range reference.TypeArguments.Nodes {
		if !l.isSendableTypeNode(argument, false, make(map[*ast.Symbol]struct{})) {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) isSendableTypeNode(node *ast.Node, shared bool, seen map[*ast.Symbol]struct{}) bool {
	for ast.IsParenthesizedTypeNode(node) {
		node = node.AsParenthesizedTypeNode().Type
	}
	if ast.IsUnionTypeNode(node) {
		for _, component := range node.AsUnionTypeNode().Types.Nodes {
			if !l.isSendableTypeNode(component, shared, seen) {
				return false
			}
		}
		return true
	}
	var symbol *ast.Symbol
	if ast.IsTypeReferenceNode(node) {
		symbol = l.trueSymbolAtLocation(node.AsTypeReferenceNode().TypeName)
	}
	if symbol != nil && symbol.Flags&ast.SymbolFlagsTypeAlias != 0 && len(symbol.Declarations) > 0 && ast.IsTypeAliasDeclaration(symbol.Declarations[0]) {
		if _, exists := seen[symbol]; exists {
			return false
		}
		seen[symbol] = struct{}{}
		return l.isSendableTypeNode(symbol.Declarations[0].Type(), shared, seen)
	}
	if symbol != nil && symbol.Flags == ast.SymbolFlagsConstEnum {
		return true
	}
	t := l.checker.getTypeFromTypeNode(node)
	if shared && t != nil && t.flags&(TypeFlagsBooleanLiteral|TypeFlagsNumberLiteral|TypeFlagsStringLiteral|TypeFlagsBigIntLiteral) != 0 && t.flags&TypeFlagsEnumLiteral == 0 {
		return true
	}
	return l.isSendableType(t)
}

func (l *arkTSLinter) handleSendableTypeParameter(node *ast.Node) {
	if node.Parent == nil || !ast.IsClassDeclaration(node.Parent) || !l.hasDecorator(node.Parent, "Sendable") || node.AsTypeParameterDeclaration().DefaultType == nil {
		return
	}
	defaultType := node.AsTypeParameterDeclaration().DefaultType
	if !l.isSendableTypeNode(defaultType, false, make(map[*ast.Symbol]struct{})) {
		l.add(defaultType, arkTSSendableGenericTypes)
	}
}

func (l *arkTSLinter) handleImportDeclaration(node *ast.Node) {
	for _, statement := range node.Parent.AsSourceFile().Statements.Nodes {
		if statement == node {
			break
		}
		if !ast.IsImportDeclaration(statement) {
			l.add(node, arkTSImportAfterStatement)
			break
		}
	}
	if node.AsImportDeclaration().Attributes != nil {
		l.add(node.AsImportDeclaration().Attributes, arkTSImportAssertion)
	}
	if l.inSharedModule() && node.AsImportDeclaration().ImportClause == nil {
		l.add(node, arkTSSharedNoSideEffectImport)
	}
}

func (l *arkTSLinter) handleFunctionExpression(node *ast.Node) {
	l.add(node, arkTSFunctionExpression)
	if arkTSAsteriskToken(node) != nil {
		l.add(node, arkTSGeneratorFunction)
	}
	l.handleStandaloneThis(node, node.Body())
	l.handleMissingReturnType(node)
}

func (l *arkTSLinter) handleFunctionDeclaration(node *ast.Node) {
	if node.Name() != nil {
		l.countDuplicateName(node.Name(), node, node.Kind)
	}
	l.handleMissingReturnType(node)
	if body := node.Body(); body != nil {
		l.reportThisKeywords(body)
	}
	if node.Parent != nil && !ast.IsSourceFile(node.Parent) && !ast.IsModuleBlock(node.Parent) {
		l.add(node, arkTSLocalFunction)
	}
	if arkTSAsteriskToken(node) != nil {
		l.add(node, arkTSGeneratorFunction)
	}
	if l.functionHasDecoratorInOverloads(node, "Sendable") {
		if !l.sendableFunctionOrAliasAvailable(node) {
			return
		}
		l.reportNonSendableDecorators(node, arkTSSendableFunctionDecorator)
		if !l.hasDecorator(node, "Sendable") {
			l.add(node, arkTSSendableFunctionOverloadDecorator)
		}
		l.scanCapturedVariables(node, node, arkTSSendableFunctionImportedVariables)
	}
}

func (l *arkTSLinter) functionHasDecoratorInOverloads(node *ast.Node, name string) bool {
	symbol := node.Symbol()
	if symbol == nil && node.Name() != nil {
		symbol = l.checker.getSymbolAtLocation(node.Name(), false)
	}
	if symbol == nil {
		return l.hasDecorator(node, name)
	}
	for _, declaration := range symbol.Declarations {
		if ast.IsFunctionDeclaration(declaration) && l.hasDecorator(declaration, name) {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) sendableFunctionOrAliasAvailable(node *ast.Node) bool {
	version := float64(12)
	if l.checker.compilerOptions.CompatibleSdkVersion != nil {
		version = *l.checker.compilerOptions.CompatibleSdkVersion
	}
	stage := l.checker.compilerOptions.CompatibleSdkVersionStage
	if stage == "" {
		stage = "beta1"
	}
	if version > 12 || version == 12 && stage != "beta1" && stage != "beta2" {
		return true
	}
	for _, decorator := range l.decorators(node) {
		if l.decoratorName(decorator) == "Sendable" {
			l.add(decorator, arkTSSendableBetaCompatible)
			break
		}
	}
	return false
}

func (l *arkTSLinter) handleSendableTypeAlias(node *ast.Node) {
	if !l.hasDecorator(node, "Sendable") {
		return
	}
	if !l.sendableFunctionOrAliasAvailable(node) {
		return
	}
	l.reportNonSendableDecorators(node, arkTSSendableTypeAliasDecorator)
	if !ast.IsFunctionTypeNode(node.Type()) {
		l.add(node.Type(), arkTSSendableTypeAliasDeclaration)
	}
}

func (l *arkTSLinter) handleStandaloneThis(node *ast.Node, body *ast.Node) {
	if ast.FindAncestor(node.Parent, func(parent *ast.Node) bool {
		return ast.IsClassLike(parent) || ast.IsInterfaceDeclaration(parent)
	}) == nil {
		l.reportThisKeywords(body)
	}
}

func (l *arkTSLinter) reportThisKeywords(scope *ast.Node) {
	var visit func(*ast.Node)
	visit = func(node *ast.Node) {
		if node == nil {
			return
		}
		if node != scope && (ast.IsClassLike(node) || ast.IsFunctionDeclaration(node) || ast.IsFunctionExpression(node) || ast.IsModuleDeclaration(node)) {
			return
		}
		if node.Kind == ast.KindThisKeyword {
			l.add(node, arkTSFunctionContainsThis)
		}
		node.ForEachChild(func(child *ast.Node) bool {
			visit(child)
			return false
		})
	}
	visit(scope)
}

func (l *arkTSLinter) handleMissingReturnType(node *ast.Node) {
	if node.Type() != nil {
		return
	}
	if ast.IsMethodSignatureDeclaration(node) || node.Body() == nil {
		if ast.IsMethodSignatureDeclaration(node) || node.Flags&ast.NodeFlagsAmbient != 0 {
			l.add(node, arkTSLimitedReturnTypeInference)
		}
		return
	}
	if l.hasCallWithOmittedReturnType(node.Body()) {
		l.add(node, arkTSLimitedReturnTypeInference)
		return
	}
	signature := l.checker.getSignatureFromDeclaration(node)
	if signature != nil {
		returnType := l.checker.getReturnTypeOfSignature(signature)
		if returnType == nil || returnType.flags&(TypeFlagsAny|TypeFlagsUnknown|TypeFlagsIntersection) != 0 {
			l.add(node, arkTSLimitedReturnTypeInference)
		}
	}
}

func (l *arkTSLinter) hasCallWithOmittedReturnType(body *ast.Node) bool {
	if body != nil && !ast.IsBlock(body) {
		expression := ast.SkipParentheses(body)
		if ast.IsCallExpression(expression) {
			signature := l.checker.getResolvedSignature(expression, nil, CheckModeNormal)
			return signature == nil || signature.declaration == nil || signature.declaration.Type() == nil
		}
	}
	found := false
	var visit func(*ast.Node)
	visit = func(node *ast.Node) {
		if node == nil || found {
			return
		}
		if node != body && (ast.IsFunctionLike(node) || ast.IsClassLike(node)) {
			return
		}
		if ast.IsReturnStatement(node) && node.Expression() != nil {
			expression := ast.SkipParentheses(node.Expression())
			if ast.IsCallExpression(expression) {
				signature := l.checker.getResolvedSignature(expression, nil, CheckModeNormal)
				if signature == nil || signature.declaration == nil || signature.declaration.Type() == nil {
					found = true
					return
				}
			}
		}
		node.ForEachChild(func(child *ast.Node) bool {
			visit(child)
			return false
		})
	}
	visit(body)
	return found
}

func (l *arkTSLinter) handlePrefixUnaryExpression(node *ast.Node) {
	expression := node.AsPrefixUnaryExpression()
	if expression.Operator != ast.KindPlusToken && expression.Operator != ast.KindMinusToken && expression.Operator != ast.KindTildeToken {
		return
	}
	typeOfOperand := l.checker.getTypeOfNode(expression.Operand)
	if typeOfOperand == nil || typeOfOperand.flags&(TypeFlagsNumberLike|TypeFlagsBigIntLike) == 0 {
		l.add(node, arkTSUnaryArithmNotNumber)
	}
}

func (l *arkTSLinter) handleBinaryExpression(node *ast.Node) {
	expression := node.AsBinaryExpression()
	operator := expression.OperatorToken.Kind
	if isAssignmentOperator(operator) {
		if ast.IsObjectLiteralExpression(expression.Left) || ast.IsArrayLiteralExpression(expression.Left) {
			l.add(node, arkTSDestructuringAssignment)
		}
		if ast.IsPropertyAccessExpression(expression.Left) {
			symbol := l.trueSymbolAtLocation(expression.Left)
			methodAssignment := l.isMethodAssignment(symbol)
			if symbol != nil && (symbol.Flags&ast.SymbolFlagsMethod != 0 || methodAssignment) {
				l.add(expression.Left, arkTSMethodReassignment)
			}
			baseSymbol := l.trueSymbolAtLocation(expression.Left.Expression())
			if methodAssignment &&
				baseSymbol != nil && baseSymbol.Flags&ast.SymbolFlagsFunction != 0 {
				l.add(expression.Left, arkTSPropertyDeclOnFunction)
			}
		}
	}
	if operator == ast.KindEqualsToken {
		leftType := l.checker.getTypeOfNode(expression.Left)
		l.checkAssignmentMatching(node, leftType, expression.Right, false)
		l.handleESObjectAssignment(node, l.variableDeclarationTypeNode(expression.Left), expression.Right, false)
	}
	switch operator {
	case ast.KindCommaToken:
		root := node
		for ast.IsBinaryExpression(root.Parent) {
			root = root.Parent
		}
		if ast.IsForStatement(root.Parent) {
			forStatement := root.Parent.AsForStatement()
			if forStatement.Initializer == root || forStatement.Incrementor == root {
				return
			}
		}
		l.add(node, arkTSCommaOperator)
	case ast.KindInKeyword:
		l.add(expression.OperatorToken, arkTSInOperator)
	case ast.KindInstanceOfKeyword:
		if expression.Left.Kind == ast.KindThisKeyword {
			return
		}
		left := ast.SkipParentheses(expression.Left)
		leftType := l.checker.getTypeOfNode(expression.Left)
		leftSymbol := l.trueSymbolAtLocation(left)
		if leftType != nil && leftType.flags&(TypeFlagsBooleanLike|TypeFlagsNumberLike) != 0 || ast.IsTypeNode(left) || leftSymbol != nil && leftSymbol.Flags&(ast.SymbolFlagsClass|ast.SymbolFlagsInterface) != 0 {
			l.add(node, arkTSInstanceofUnsupported)
		}
	}
}

// OpenHarmony's TypeScript binder marks function-valued expando assignments
// as Method|Assignment. The current TypeScript binder deliberately represents
// the same declaration as Property|Assignment. Accept both representations so
// the source-defined linter rule does not alter ordinary TypeScript symbols.
func (l *arkTSLinter) isMethodAssignment(symbol *ast.Symbol) bool {
	if symbol == nil || symbol.Flags&ast.SymbolFlagsAssignment == 0 {
		return false
	}
	if symbol.Flags&ast.SymbolFlagsMethod != 0 {
		return true
	}
	declaration := symbol.ValueDeclaration
	if declaration == nil && len(symbol.Declarations) != 0 {
		declaration = symbol.Declarations[0]
	}
	return declaration != nil && ast.IsBinaryExpression(declaration) && ast.IsFunctionLikeDeclaration(declaration.AsBinaryExpression().Right)
}

func isAssignmentOperator(kind ast.Kind) bool {
	return kind >= ast.KindFirstAssignment && kind <= ast.KindLastAssignment
}

func arkTSAsteriskToken(node *ast.Node) *ast.Node {
	switch node.Kind {
	case ast.KindFunctionDeclaration:
		return node.AsFunctionDeclaration().AsteriskToken
	case ast.KindFunctionExpression:
		return node.AsFunctionExpression().AsteriskToken
	case ast.KindMethodDeclaration:
		return node.AsMethodDeclaration().AsteriskToken
	}
	return nil
}

func (l *arkTSLinter) handleVariableDeclaration(node *ast.Node) {
	name := node.Name()
	if ast.IsArrayBindingPattern(name) || ast.IsObjectBindingPattern(name) {
		l.add(node, arkTSDestructuringDeclaration)
	}
	l.visitBindingNames(name, func(identifier *ast.Node) {
		l.countDuplicateName(identifier, identifier, identifier.Parent.Kind)
	})
	l.handleInferredDeclaration(node)
	l.handleDefiniteAssignment(node)
	if node.Type() != nil && node.Initializer() != nil {
		l.checkAssignmentMatching(node, l.checker.getTypeOfNode(node.Type()), node.Initializer(), false)
	}
	if node.Initializer() != nil {
		l.handleESObjectAssignment(node, node.Type(), node.Initializer(), false)
	}
}

func (l *arkTSLinter) handlePropertyAssignmentCompatibility(node *ast.Node) {
	if node.Type() == nil || node.Initializer() == nil {
		return
	}
	l.checkAssignmentMatching(node, l.checker.getTypeOfNode(node.Type()), node.Initializer(), true)
	l.handleESObjectAssignment(node, node.Type(), node.Initializer(), true)
}

func (l *arkTSLinter) handleInferredDeclaration(node *ast.Node) {
	if node.Type() != nil || ast.IsCatchClause(node.Parent) || ast.IsBindingPattern(node.Name()) {
		return
	}
	initializer := node.Initializer()
	if initializer == nil && (ast.IsPropertyDeclaration(node) || ast.IsVariableStatement(node.Parent.Parent)) {
		if ast.IsPropertyDeclaration(node) && l.file.IsDeclarationFile && l.file.ScriptKind == core.ScriptKindETS && node.ModifierFlags()&ast.ModifierFlagsPrivate != 0 {
			return
		}
		l.add(node, arkTSAnyType)
		return
	}
	t := l.checker.getTypeOfNode(node)
	if l.containsAnyOrUnknown(t, make(map[*Type]struct{})) {
		if t != nil && t.flags&TypeFlagsUnknown != 0 {
			l.add(node, arkTSUnknownType)
		} else {
			l.add(node, arkTSAnyType)
		}
	}
}

func (l *arkTSLinter) containsAnyOrUnknown(t *Type, visited map[*Type]struct{}) bool {
	if t == nil || t.alias != nil {
		return false
	}
	if _, exists := visited[t]; exists {
		return false
	}
	visited[t] = struct{}{}
	if t.flags&(TypeFlagsAny|TypeFlagsUnknown) != 0 {
		return true
	}
	if t.IsUnion() || t.IsIntersection() {
		for _, part := range t.Types() {
			if l.containsAnyOrUnknown(part, visited) {
				return true
			}
		}
	}
	if t.objectFlags&ObjectFlagsReference != 0 {
		for _, argument := range l.checker.getTypeArguments(t) {
			if l.containsAnyOrUnknown(argument, visited) {
				return true
			}
		}
	}
	return false
}

func (l *arkTSLinter) handleDefiniteAssignment(node *ast.Node) {
	var token *ast.Node
	if ast.IsVariableDeclaration(node) {
		token = node.AsVariableDeclaration().ExclamationToken
	} else if ast.IsPropertyDeclaration(node) {
		token = node.AsPropertyDeclaration().PostfixToken
		if token != nil && token.Kind != ast.KindExclamationToken {
			token = nil
		}
	}
	if token == nil {
		return
	}
	if ast.IsPropertyDeclaration(node) && ast.IsClassDeclaration(node.Parent) && l.hasDecorator(node.Parent, "Sendable") {
		l.add(node, arkTSSendableDefiniteAssignment)
	} else {
		l.add(node, arkTSDefiniteAssignment)
	}
}

func (l *arkTSLinter) handleClassDeclaration(node *ast.Node) {
	if node.Name() != nil {
		l.countDuplicateName(node.Name(), node, node.Kind)
	}
	l.countClassMemberDuplicateNames(node)
	// ArkTS structs share the class-shaped Go AST representation, while the OH
	// linter's isClassDeclaration predicate excludes SyntaxKind.StructDeclaration.
	isSendableClass := !ast.IsStructDeclaration(node) && l.hasDecorator(node, "Sendable")
	if isSendableClass {
		l.reportNonSendableDecorators(node, arkTSSendableClassDecorator)
	}
	if l.checker.compilerOptions.SkipArkTSStaticBlocksCheck != core.TSTrue {
		seen := false
		for _, member := range node.Members() {
			if ast.IsClassStaticBlockDeclaration(member) {
				if seen {
					l.add(member, arkTSMultipleStaticBlocks)
				}
				seen = true
			}
		}
	}
	var heritageClauses *ast.NodeList
	if ast.IsClassDeclaration(node) {
		heritageClauses = node.AsClassDeclaration().HeritageClauses
	}
	if heritageClauses != nil {
		for _, clause := range heritageClauses.Nodes {
			heritageClause := clause.AsHeritageClause()
			for _, heritageType := range heritageClause.Types.Nodes {
				t := l.reduceReference(l.checker.getTypeOfNode(heritageType))
				baseSendable := l.isSendableClassOrInterface(t)
				if heritageClause.Token == ast.KindImplementsKeyword && t != nil && t.IsClass() {
					l.add(heritageType, arkTSImplementsClass)
				}
				if !isSendableClass {
					if baseSendable {
						l.add(heritageType, arkTSSendableClassInheritance)
					}
					continue
				}
				if heritageClause.Token == ast.KindExtendsKeyword && (!baseSendable || !l.isValidSendableClassExtends(heritageType)) {
					l.add(heritageType, arkTSSendableClassInheritance)
				}
			}
		}
	}
	if isSendableClass {
		for _, member := range node.Members() {
			l.scanCapturedVariables(member, node, arkTSSendableCapturedVars)
		}
	}
}

func (l *arkTSLinter) isValidSendableClassExtends(heritageType *ast.Node) bool {
	expression := arkTSHeritageExpression(heritageType)
	symbol := l.checker.getSymbolAtLocation(expression, false)
	if symbol == nil || symbol.Flags&ast.SymbolFlagsClass != 0 {
		return true
	}
	if symbol.Flags&ast.SymbolFlagsAlias != 0 {
		real := l.checker.resolveAlias(symbol)
		return real != nil && real.Flags&ast.SymbolFlagsClass != 0
	}
	return false
}

func (l *arkTSLinter) scanCapturedVariables(start *ast.Node, scope *ast.Node, fault arkTSFault) {
	var visit func(*ast.Node)
	visit = func(node *ast.Node) {
		if node == nil {
			return
		}
		if ast.IsIdentifier(node) {
			rawSymbol := l.checker.getSymbolAtLocation(node, false)
			if rawSymbol != nil && len(rawSymbol.Declarations) > 0 && ast.IsNamespaceImport(rawSymbol.Declarations[0]) {
				l.add(node, fault)
				return
			}
			if node.Parent == nil || !ast.IsPropertyAccessExpression(node.Parent) || node.Parent.Name() != node {
				if !ast.IsFunctionDeclaration(start) || start.Name() != node {
					l.checkCapturedIdentifier(node, scope, fault)
				}
			}
		}
		if ast.IsTypeReferenceNode(node) {
			// OH heritage entries are ExpressionWithTypeArguments nodes, so their
			// expression is scanned for captured declarations. TSGO represents the
			// same ArkTS syntax as a TypeReferenceNode; visit only its type name to
			// preserve the upstream traversal without scanning type arguments.
			if node.Parent != nil && ast.IsHeritageClause(node.Parent) {
				visit(node.AsTypeReferenceNode().TypeName)
			}
			return
		}
		if ast.IsDecorator(node) && node.Parent == start {
			return
		}
		node.ForEachChild(func(child *ast.Node) bool {
			visit(child)
			return false
		})
	}
	visit(start)
}

func (l *arkTSLinter) checkCapturedIdentifier(node *ast.Node, scope *ast.Node, fault arkTSFault) {
	symbol := l.trueSymbolAtLocation(node)
	if symbol == nil || symbol.Flags == ast.SymbolFlagsConstEnum || len(symbol.Declarations) == 0 {
		return
	}
	declaration := symbol.Declarations[0]
	declarationFile := ast.GetSourceFileOfNode(declaration)
	if declarationFile == nil || declarationFile.FileName() != l.file.FileName() {
		return
	}
	position := scanner.GetTokenPosOfNode(declaration, declarationFile, false)
	scopeStart := scanner.GetTokenPosOfNode(scope, l.file, false)
	if position >= scopeStart && position < scope.End() || l.isExportedDeclaration(declaration) || l.isAllowedTopLevelSendableClosure(declaration) {
		return
	}
	switch declaration.Kind {
	case ast.KindVariableDeclaration, ast.KindFunctionDeclaration, ast.KindClassDeclaration, ast.KindInterfaceDeclaration,
		ast.KindEnumDeclaration, ast.KindModuleDeclaration, ast.KindParameter:
		l.add(node, fault)
	}
}

func (l *arkTSLinter) isAllowedTopLevelSendableClosure(declaration *ast.Node) bool {
	if declaration.Parent == nil || !ast.IsSourceFile(declaration.Parent) {
		return false
	}
	if ast.IsClassDeclaration(declaration) && l.isSendableClassOrInterface(l.checker.getTypeOfNode(declaration)) {
		return true
	}
	return ast.IsFunctionDeclaration(declaration) && l.functionHasDecoratorInOverloads(declaration, "Sendable")
}

func (l *arkTSLinter) isExportedDeclaration(declaration *ast.Node) bool {
	if l.exported == nil {
		l.buildExportedDeclarations()
	}
	_, exists := l.exported[declaration]
	return exists
}

func (l *arkTSLinter) buildExportedDeclarations() {
	l.exported = make(map[*ast.Node]struct{})
	addSymbolDeclarations := func(node *ast.Node) {
		symbol := l.trueSymbolAtLocation(node)
		if symbol != nil {
			for _, declaration := range symbol.Declarations {
				l.exported[declaration] = struct{}{}
			}
		}
	}
	for _, statement := range l.file.Statements.Nodes {
		switch statement.Kind {
		case ast.KindExportAssignment:
			if !statement.AsExportAssignment().IsExportEquals {
				addSymbolDeclarations(statement.Expression())
			}
		case ast.KindExportDeclaration:
			clause := statement.AsExportDeclaration().ExportClause
			if clause == nil || !ast.IsNamedExports(clause) {
				continue
			}
			for _, specifier := range clause.AsNamedExports().Elements.Nodes {
				name := specifier.AsExportSpecifier().PropertyName
				if name == nil {
					name = specifier.Name()
				}
				addSymbolDeclarations(name)
			}
		default:
			if statement.ModifierFlags()&ast.ModifierFlagsExport == 0 {
				continue
			}
			if ast.IsVariableStatement(statement) {
				for _, declaration := range statement.AsVariableStatement().DeclarationList.AsVariableDeclarationList().Declarations.Nodes {
					l.exported[declaration] = struct{}{}
				}
			} else {
				l.exported[statement] = struct{}{}
			}
		}
	}
}

func (l *arkTSLinter) reduceReference(t *Type) *Type {
	if t != nil && t.flags&TypeFlagsObject != 0 && t.objectFlags&ObjectFlagsReference != 0 && t.AsObjectType().target != nil && t.AsObjectType().target != t {
		return t.AsObjectType().target
	}
	return t
}

func (l *arkTSLinter) handleInterfaceDeclaration(node *ast.Node) {
	symbol := l.checker.getSymbolAtLocation(node.Name(), false)
	if l.countDeclarationKind(symbol, ast.KindInterfaceDeclaration) > 1 {
		l.add(node, arkTSInterfaceMerging)
	}
	l.countDuplicateName(node.Name(), node, node.Kind)
	heritageClauses := node.AsInterfaceDeclaration().HeritageClauses
	if heritageClauses == nil {
		return
	}
	for _, clause := range heritageClauses.Nodes {
		heritageClause := clause.AsHeritageClause()
		if heritageClause.Token != ast.KindExtendsKeyword {
			continue
		}
		propertyTypes := make(map[string]string)
		for _, heritageType := range heritageClause.Types.Nodes {
			// The Go AST stores an ETS heritage entry as a TypeReference rather
			// than the upstream ExpressionWithTypeArguments wrapper. Querying the
			// full node is the equivalent of OH getTypeAtLocation(expression).
			t := l.checker.getTypeOfNode(heritageType)
			if t != nil && t.IsClass() {
				l.add(heritageType, arkTSInterfaceExtendsClass)
			} else if t != nil && t.objectFlags&ObjectFlagsClassOrInterface != 0 {
				l.checkInheritedPropertyTypes(node, t, propertyTypes)
			}
		}
	}
}

func arkTSHeritageExpression(node *ast.Node) *ast.Node {
	if node == nil {
		return nil
	}
	if ast.IsExpressionWithTypeArguments(node) {
		return node.Expression()
	}
	if ast.IsTypeReferenceNode(node) {
		return node.AsTypeReferenceNode().TypeName
	}
	return node
}

// checkInheritedPropertyTypes follows
// TypeScriptLinter.ts::lintForInterfaceExtendsDifferentPorpertyTypes. The
// comparison is intentionally on the declared type-node text, not checker type
// identity: this is the observable upstream rule (for example aliases with
// different spellings remain different).
func (l *arkTSLinter) checkInheritedPropertyTypes(interfaceNode *ast.Node, inherited *Type, propertyTypes map[string]string) {
	for _, property := range l.checker.getPropertiesOfType(inherited) {
		if len(property.Declarations) == 0 {
			continue
		}
		declaration := property.Declarations[0]
		switch declaration.Kind {
		case ast.KindMethodSignature, ast.KindMethodDeclaration, ast.KindPropertyDeclaration, ast.KindPropertySignature:
		default:
			continue
		}
		typeNode := declaration.Type()
		if typeNode == nil {
			continue
		}
		text := arkTSNodeText(typeNode)
		if previous, exists := propertyTypes[property.Name]; !exists {
			propertyTypes[property.Name] = text
		} else if previous != text {
			l.add(interfaceNode, arkTSInterfaceExtendDifferentProperties)
		}
	}
}

func arkTSNodeText(node *ast.Node) string {
	file := ast.GetSourceFileOfNode(node)
	if file == nil {
		return ""
	}
	start := scanner.GetTokenPosOfNode(node, file, false)
	if start < 0 || node.End() < start || node.End() > len(file.Text()) {
		return ""
	}
	return file.Text()[start:node.End()]
}

func (l *arkTSLinter) handleEnumDeclaration(node *ast.Node) {
	symbol := l.checker.getSymbolAtLocation(node.Name(), false)
	l.countDuplicateName(node.Name(), node, node.Kind)
	if l.countDeclarationKind(symbol, ast.KindEnumDeclaration) > 1 {
		l.add(node, arkTSEnumMerging)
	}
}

func (l *arkTSLinter) countDeclarationKind(symbol *ast.Symbol, kind ast.Kind) int {
	count := 0
	if symbol != nil {
		for _, declaration := range symbol.Declarations {
			if declaration.Kind == kind {
				count++
			}
		}
	}
	return count
}

func (l *arkTSLinter) handleModuleDeclaration(node *ast.Node) {
	declaration := node.AsModuleDeclaration()
	l.countDuplicateName(node.Name(), node, node.Kind)
	if declaration.Body != nil && ast.IsModuleBlock(declaration.Body) {
		for _, statement := range declaration.Body.AsModuleBlock().Statements.Nodes {
			switch statement.Kind {
			case ast.KindVariableStatement, ast.KindFunctionDeclaration, ast.KindClassDeclaration, ast.KindInterfaceDeclaration,
				ast.KindTypeAliasDeclaration, ast.KindEnumDeclaration, ast.KindExportDeclaration, ast.KindModuleDeclaration:
			default:
				l.add(statement, arkTSNonDeclarationInNamespace)
			}
		}
	}
	if declaration.Keyword != ast.KindNamespaceKeyword && node.ModifierFlags()&ast.ModifierFlagsAmbient != 0 {
		l.add(node, arkTSShorthandAmbientModuleDecl)
	}
	if ast.IsStringLiteral(node.Name()) && strings.Contains(node.Name().Text(), "*") {
		l.add(node, arkTSWildcardsInModuleName)
	}
}

func (l *arkTSLinter) countDuplicateName(name *ast.Node, declaration *ast.Node, declarationKind ast.Kind) {
	// TypeScriptLinter.ts::countDeclarationsWithDuplicateName deliberately uses
	// getSymbolAtLocation here, not trueSymbolAtLocation. Following an import
	// alias would compare the imported declaration kind with the local import
	// binding and report every ordinary import as a duplicate.
	symbol := l.checker.getSymbolAtLocation(name, false)
	if symbol != nil && l.symbolHasDuplicateName(symbol, declarationKind) {
		l.add(declaration, arkTSDeclWithDuplicateName)
	}
}

func (l *arkTSLinter) visitBindingNames(name *ast.Node, visit func(*ast.Node)) {
	if name == nil {
		return
	}
	if ast.IsIdentifier(name) {
		visit(name)
		return
	}
	if !ast.IsBindingPattern(name) {
		return
	}
	name.ForEachChild(func(child *ast.Node) bool {
		if ast.IsBindingElement(child) {
			l.visitBindingNames(child.Name(), visit)
		}
		return false
	})
}

func (l *arkTSLinter) countClassMemberDuplicateNames(class *ast.Node) {
	members := class.Members()
	for _, current := range members {
		currentName := current.Name()
		if currentName == nil || !ast.IsIdentifier(currentName) && !ast.IsPrivateIdentifier(currentName) {
			continue
		}
		currentText := strings.TrimPrefix(currentName.Text(), "#")
		for _, other := range members {
			if current == other {
				continue
			}
			otherName := other.Name()
			if otherName == nil || !ast.IsIdentifier(otherName) && !ast.IsPrivateIdentifier(otherName) || ast.IsPrivateIdentifier(currentName) == ast.IsPrivateIdentifier(otherName) {
				continue
			}
			if currentText == strings.TrimPrefix(otherName.Text(), "#") {
				l.add(current, arkTSDeclWithDuplicateName)
				break
			}
		}
	}
}

func (l *arkTSLinter) isAllowedIndexSignature(node *ast.Node) bool {
	parameters := node.Parameters()
	if len(parameters) != 1 {
		return false
	}
	parameterType := l.checker.getTypeOfNode(parameters[0])
	if parameterType == nil || parameterType.flags&TypeFlagsNumber == 0 {
		return false
	}
	if node.Parent == nil || node.Parent.Name() == nil {
		return false
	}
	symbol := l.trueSymbolAtLocation(node.Parent.Name())
	return symbol != nil && l.isArkTSCollectionsDeclaration(symbol.Declarations)
}

func (l *arkTSLinter) handleIdentifier(node *ast.Node) {
	symbol := l.trueSymbolAtLocation(node)
	if symbol == nil {
		return
	}
	if node.Text() == "globalThis" {
		if symbol != nil && symbol.Flags&ast.SymbolFlagsModule != 0 && symbol.Flags&ast.SymbolFlagsTransient != 0 {
			l.add(node, arkTSGlobalThis)
			return
		}
	}
	l.handleRestrictedValue(node, symbol)
}

func (l *arkTSLinter) handleRestrictedValue(identifier *ast.Node, symbol *ast.Symbol) {
	illegal := ast.SymbolFlagsConstEnum | ast.SymbolFlagsRegularEnum | ast.SymbolFlagsValueModule | ast.SymbolFlagsClass
	if symbol.Flags&illegal == 0 || l.isStructSymbol(symbol) || !l.identifierUsedInValueContext(identifier, symbol) {
		return
	}
	if symbol.Flags&ast.SymbolFlagsValueModule != 0 && l.symbolHasDuplicateName(symbol, ast.KindModuleDeclaration) {
		return
	}
	if symbol.Flags&ast.SymbolFlagsClass != 0 && l.isAllowedClassValueContext(identifier) {
		return
	}
	if symbol.Flags&ast.SymbolFlagsAnnotation != 0 {
		return
	}
	if symbol.Flags&ast.SymbolFlagsValueModule != 0 {
		l.add(identifier, arkTSNamespaceAsObject)
	} else {
		l.add(identifier, arkTSClassAsObject)
	}
}

func (l *arkTSLinter) identifierUsedInValueContext(identifier *ast.Node, symbol *ast.Symbol) bool {
	root := identifier
	for root.Parent != nil && (ast.IsPropertyAccessExpression(root.Parent) || ast.IsQualifiedName(root.Parent)) {
		root = root.Parent
	}
	parent := root.Parent
	if parent == nil {
		return false
	}
	if ast.IsTypeNode(parent) && !ast.IsTypeOfExpression(parent) ||
		ast.IsExpressionWithTypeArguments(parent) || ast.IsExportAssignment(parent) || ast.IsExportSpecifier(parent) ||
		ast.IsMetaProperty(parent) || ast.IsImportClause(parent) || ast.IsClassLike(parent) || ast.IsInterfaceDeclaration(parent) ||
		ast.IsModuleDeclaration(parent) || ast.IsEnumDeclaration(parent) || ast.IsNamespaceImport(parent) || ast.IsImportSpecifier(parent) ||
		ast.IsImportEqualsDeclaration(parent) || ast.IsQualifiedName(root) && identifier != root.AsQualifiedName().Right ||
		ast.IsPropertyAccessExpression(root) && identifier != root.Name() || ast.IsNewExpression(parent) && parent.Expression() == root ||
		ast.IsBinaryExpression(parent) && parent.AsBinaryExpression().OperatorToken.Kind == ast.KindInstanceOfKeyword {
		return false
	}
	if ast.IsElementAccessExpression(parent) && symbol.Flags&ast.SymbolFlagsEnum != 0 {
		expression := parent.Expression()
		if expression == identifier || ast.IsPropertyAccessExpression(expression) && expression.Name() == identifier {
			return false
		}
	}
	return true
}

func (l *arkTSLinter) isAllowedClassValueContext(identifier *ast.Node) bool {
	root := identifier
	for root.Parent != nil && (ast.IsPropertyAccessExpression(root.Parent) || ast.IsQualifiedName(root.Parent)) {
		root = root.Parent
	}
	if root.Parent != nil && ast.IsPropertyAssignment(root.Parent) && root.Parent.Parent != nil && ast.IsObjectLiteralExpression(root.Parent.Parent) {
		root = root.Parent.Parent
	}
	if root.Parent != nil && ast.IsArrowFunction(root.Parent) && root.Parent.Body() == root {
		root = root.Parent
	}
	if root.Parent == nil || !ast.IsCallExpression(root.Parent) && !ast.IsNewExpression(root.Parent) {
		return false
	}
	callee := root.Parent.Expression()
	if callee == root {
		return false
	}
	t := l.checker.getTypeOfNode(callee)
	return IsTypeAny(t) || l.isLibraryType(t)
}

func (l *arkTSLinter) isStructSymbol(symbol *ast.Symbol) bool {
	for _, declaration := range symbol.Declarations {
		if ast.IsStructDeclaration(declaration) {
			return true
		}
	}
	return false

}

func (l *arkTSLinter) symbolHasDuplicateName(symbol *ast.Symbol, declarationKind ast.Kind) bool {
	for _, declaration := range symbol.Declarations {
		kind := declaration.Kind
		isTypeDeclaration := func(kind ast.Kind) bool {
			return kind == ast.KindEnumDeclaration || kind == ast.KindClassDeclaration || kind == ast.KindInterfaceDeclaration || kind == ast.KindTypeAliasDeclaration
		}
		namespaceTypeCollision := isTypeDeclaration(kind) && declarationKind == ast.KindModuleDeclaration || isTypeDeclaration(declarationKind) && kind == ast.KindModuleDeclaration
		if kind != ast.KindIdentifier && kind != declarationKind && !namespaceTypeCollision {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) handlePropertyAccess(node *ast.Node) {
	if ast.IsCallExpression(node.Parent) && node.Parent.Expression() == node {
		return
	}
	if node.Name().Text() == "prototype" {
		base := l.checker.getTypeOfNode(node.Expression())
		symbol := l.checker.getSymbolAtLocation(node.Expression(), false)
		if (symbol != nil && symbol.Flags&(ast.SymbolFlagsClass|ast.SymbolFlagsFunction) != 0) || IsTypeAny(base) {
			l.add(node.Name(), arkTSPrototype)
		}
	}
	symbol := l.trueSymbolAtLocation(node)
	if l.isSymbolAPI(symbol) && symbol.Name != "iterator" {
		l.add(node, arkTSSymbolType)
	}
	baseType := l.checker.getTypeOfNode(node.Expression())
	if l.isSendableFunction(baseType) || l.hasSendableTypeAlias(baseType) {
		l.add(node, arkTSSendableFunctionProperty)
	}
	if node.Parent != nil && ast.IsNewExpression(node.Parent) && l.isTaskpoolAPI(symbol, node) {
		l.handleTaskpoolNew(node.Parent)
	}
}

func (l *arkTSLinter) handleElementAccess(node *ast.Node) {
	expression := node.AsElementAccessExpression()
	baseSymbol := l.trueSymbolAtLocation(expression.Expression)
	baseType := l.typeOrConstraint(expression.Expression)
	baseType = l.checker.GetNonNullableType(baseType)
	argumentType := l.checker.getTypeOfNode(expression.ArgumentExpression)
	if l.isLibrarySymbol(baseSymbol) || ast.IsArrayLiteralExpression(expression.Expression) || l.isElementAccessAllowed(baseType, argumentType) {
		return
	}
	l.add(node, arkTSPropertyAccessByIndex)
}

func (l *arkTSLinter) isElementAccessAllowed(t *Type, argumentType *Type) bool {
	if t == nil {
		return true
	}
	if t.IsUnion() {
		for _, component := range t.Types() {
			if !l.isElementAccessAllowed(component, argumentType) {
				return false
			}
		}
		return true
	}
	if l.isOrDerivedFrom(t, func(candidate *Type) bool {
		return candidate != nil && candidate.symbol != nil && candidate.symbol.Name == "BitVector" && l.isArkTSCollectionsDeclaration(candidate.symbol.Declarations)
	}) {
		return argumentType != nil && argumentType.flags&TypeFlagsNumberLike != 0
	}
	return IsTypeAny(t) || l.isLibraryType(t) || l.isOrDerivedFrom(t, l.isIndexableArrayType) || l.isOrDerivedFrom(t, func(candidate *Type) bool { return candidate.IsTupleType() }) ||
		l.isOrDerivedFrom(t, l.isStdRecordType) || t.flags&(TypeFlagsString|TypeFlagsEnumLike|TypeFlagsNonPrimitive) != 0 || l.isNamedGlobalType(t, "Map") || l.isESObjectType(t)
}

func (l *arkTSLinter) typeOrConstraint(node *ast.Node) *Type {
	t := l.checker.getTypeOfNode(node)
	if t != nil && t.flags&TypeFlagsTypeParameter != 0 {
		if constraint := l.checker.getBaseConstraintOfType(t); constraint != nil {
			return constraint
		}
	}
	return t
}

func (l *arkTSLinter) handleCallArgumentAssignments(call *ast.Node, signature *Signature) {
	arguments := call.Arguments()
	if signature == nil || len(arguments) == 0 || len(signature.parameters) == 0 {
		return
	}
	for index, argument := range arguments {
		parameterIndex := min(index, len(signature.parameters)-1)
		parameter := signature.parameters[parameterIndex]
		declaration := parameter.ValueDeclaration
		if declaration == nil || !ast.IsParameterDeclaration(declaration) {
			continue
		}
		parameterType := l.checker.GetTypeOfSymbolAtLocation(parameter, declaration)
		if declaration.AsParameterDeclaration().DotDotDotToken != nil {
			if element := l.checker.getElementTypeOfArrayType(parameterType); element != nil {
				parameterType = element
			}
		}
		l.checkAssignmentMatching(argument, parameterType, argument, false)
	}
}

func (l *arkTSLinter) handleReturnStatement(node *ast.Node) {
	expression := node.Expression()
	if expression == nil {
		return
	}
	if contextualType := l.checker.getContextualType(expression, ContextFlagsNone); contextualType != nil {
		l.checkAssignmentMatching(node, contextualType, expression, true)
	}
}

func (l *arkTSLinter) checkAssignmentMatching(field *ast.Node, leftType *Type, rightExpression *ast.Node, missedStructural bool) {
	if leftType == nil || rightExpression == nil {
		return
	}
	rightType := l.checker.getTypeOfNode(rightExpression)
	if rightType == nil {
		return
	}
	if l.isWrongSendableFunctionAssignment(leftType, rightType) {
		l.add(field, arkTSSendableFunctionAssignment)
	}
	strict := l.needsStrictStructuralMatch(leftType, rightType)
	if missedStructural && !strict {
		return
	}
	if l.needsStructuralIdentity(leftType, rightType, rightExpression, strict) {
		l.add(field, arkTSStructuralIdentity)
	}
}

func (l *arkTSLinter) needsStructuralIdentity(leftType *Type, rightType *Type, rightExpression *ast.Node, strict bool) bool {
	leftType = l.checker.GetNonNullableType(leftType)
	rightType = l.checker.GetNonNullableType(rightType)
	if leftType == nil || rightType == nil || l.isLibraryType(leftType) || l.isDynamicAssignedToStandardType(leftType, rightExpression) || l.areCompatibleFunctionals(leftType, rightType) {
		return false
	}
	if rightType.IsUnion() {
		for _, component := range rightType.Types() {
			if l.needsStructuralIdentity(leftType, component, rightExpression, strict) {
				return true
			}
		}
		return false
	}
	if leftType.IsUnion() {
		for _, component := range leftType.Types() {
			if !l.needsStructuralIdentity(component, rightType, rightExpression, strict) {
				return false
			}
		}
		return true
	}
	if strict {
		if rightType.objectFlags&ObjectFlagsReference != 0 && rightType.objectFlags&ObjectFlagsArrayLiteral != 0 {
			return false
		}
		leftType = l.reduceReference(leftType)
		rightType = l.reduceReference(rightType)
	}
	return leftType != nil && rightType != nil && leftType.objectFlags&ObjectFlagsClassOrInterface != 0 && rightType.objectFlags&ObjectFlagsClassOrInterface != 0 &&
		!l.relatedByInheritanceOrIdentical(rightType, leftType, make(map[[2]*Type]struct{}))
}

func (l *arkTSLinter) relatedByInheritanceOrIdentical(candidate *Type, target *Type, seen map[[2]*Type]struct{}) bool {
	candidate = l.reduceReference(candidate)
	target = l.reduceReference(target)
	if candidate == nil || target == nil {
		return false
	}
	if candidate == target || candidate.symbol != nil && candidate.symbol == target.symbol || l.isNamedGlobalType(target, "Object") || target.flags&TypeFlagsNonPrimitive != 0 {
		return true
	}
	key := [2]*Type{candidate, target}
	if _, exists := seen[key]; exists {
		return false
	}
	seen[key] = struct{}{}
	if candidate.symbol == nil {
		return false
	}
	targetIsSendable := l.isISendableInterface(target)
	for _, declaration := range candidate.symbol.Declarations {
		if targetIsSendable && ast.IsClassDeclaration(declaration) && l.hasDecorator(declaration, "Sendable") {
			return true
		}
		if !ast.IsClassDeclaration(declaration) && !ast.IsInterfaceDeclaration(declaration) {
			continue
		}
		var clauses *ast.NodeList
		if ast.IsClassDeclaration(declaration) {
			clauses = declaration.AsClassDeclaration().HeritageClauses
		} else {
			clauses = declaration.AsInterfaceDeclaration().HeritageClauses
		}
		if clauses == nil {
			continue
		}
		for _, clauseNode := range clauses.Nodes {
			clause := clauseNode.AsHeritageClause()
			processInterfaces := !candidate.IsClass() || clause.Token != ast.KindExtendsKeyword
			for _, heritage := range clause.Types.Nodes {
				base := l.reduceReference(l.checker.getTypeOfNode(heritage))
				if base != nil && base.IsClass() != processInterfaces && l.relatedByInheritanceOrIdentical(base, target, seen) {
					return true
				}
			}
		}
	}
	return false
}

func (l *arkTSLinter) areCompatibleFunctionals(leftType *Type, rightType *Type) bool {
	return len(l.checker.getSignaturesOfType(leftType, SignatureKindCall)) > 0 && len(l.checker.getSignaturesOfType(rightType, SignatureKindCall)) > 0
}

func (l *arkTSLinter) isDynamicAssignedToStandardType(leftType *Type, rightExpression *ast.Node) bool {
	if !l.isStandardLibraryType(leftType) && leftType.flags&(TypeFlagsBooleanLike|TypeFlagsNumberLike|TypeFlagsStringLike|TypeFlagsBigIntLike|TypeFlagsESSymbolLike) == 0 {
		return false
	}
	var symbol *ast.Symbol
	if ast.IsCallExpression(rightExpression) {
		symbol = l.trueSymbolAtLocation(rightExpression.Expression())
	} else {
		symbol = l.trueSymbolAtLocation(rightExpression)
	}
	return l.isLibrarySymbol(symbol)
}

func (l *arkTSLinter) isStandardLibraryType(t *Type) bool {
	if t == nil || t.symbol == nil || len(t.symbol.Declarations) == 0 {
		return false
	}
	file := ast.GetSourceFileOfNode(t.symbol.Declarations[0])
	return file != nil && l.checker.program.IsSourceFileDefaultLibrary(file.Path())
}

func (l *arkTSLinter) needsStrictStructuralMatch(leftType *Type, rightType *Type) bool {
	strictLeft := false
	if leftType.IsUnion() {
		for _, component := range leftType.Types() {
			component = l.reduceReference(component)
			if component == nil || component.objectFlags&ObjectFlagsClassOrInterface == 0 {
				continue
			}
			if !l.isSendableClassOrInterface(component) {
				return false
			}
			strictLeft = true
		}
	} else {
		strictLeft = l.isSendableClassOrInterface(leftType)
	}
	return strictLeft && l.containsNonSendableClassOrInterface(rightType)
}

func (l *arkTSLinter) containsNonSendableClassOrInterface(t *Type) bool {
	if t == nil {
		return false
	}
	if t.IsUnion() {
		for _, component := range t.Types() {
			if l.containsNonSendableClassOrInterface(component) {
				return true
			}
		}
		return false
	}
	t = l.reduceReference(t)
	return t != nil && t.objectFlags&ObjectFlagsClassOrInterface != 0 && !l.isSendableClassOrInterface(t)
}

func (l *arkTSLinter) isIndexableArrayType(t *Type) bool {
	if t == nil || t.symbol == nil {
		return false
	}
	switch t.symbol.Name {
	case "Array", "ReadonlyArray", "ConcatArray", "ArrayLike", "Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array", "Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array":
		return true
	case "BitVector":
		return l.isArkTSCollectionsDeclaration(t.symbol.Declarations)
	}
	return false
}

func (l *arkTSLinter) isSpreadArrayType(t *Type) bool {
	if t == nil || t.symbol == nil {
		return false
	}
	switch t.symbol.Name {
	case "Array", "ReadonlyArray", "Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array", "Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array":
		return true
	case "BitVector":
		return l.isArkTSCollectionsDeclaration(t.symbol.Declarations)
	}
	return false
}

func (l *arkTSLinter) isArkTSCollectionsDeclaration(declarations []*ast.Node) bool {
	if len(declarations) == 0 {
		return false
	}
	declaration := declarations[0]
	file := ast.GetSourceFileOfNode(declaration)
	return (ast.IsClassDeclaration(declaration) || ast.IsInterfaceDeclaration(declaration)) && declaration.Parent != nil && ast.IsModuleBlock(declaration.Parent) && declaration.Parent.Parent != nil && declaration.Parent.Parent.Name().Text() == "collections" && file != nil && strings.EqualFold(tspath.GetBaseFileName(file.FileName()), "@arkts.collections.d.ets")
}

func (l *arkTSLinter) isOrDerivedFrom(t *Type, predicate func(*Type) bool) bool {
	seen := make(map[*Type]struct{})
	var visit func(*Type) bool
	visit = func(current *Type) bool {
		current = l.reduceReference(current)
		if current == nil {
			return false
		}
		if _, exists := seen[current]; exists {
			return false
		}
		seen[current] = struct{}{}
		if predicate(current) {
			return true
		}
		for _, base := range l.checker.getBaseTypes(current) {
			if visit(base) {
				return true
			}
		}
		return false
	}
	return visit(t)
}

func (l *arkTSLinter) isNamedGlobalType(t *Type, name string) bool {
	return t != nil && t.symbol != nil && t.symbol.Name == name && l.checker.GetFullyQualifiedName(t.symbol) == name
}

func (l *arkTSLinter) isESObjectType(t *Type) bool {
	return t != nil && t.alias != nil && t.alias.Symbol() != nil && t.alias.Symbol().Name == "ESObject"
}

func (l *arkTSLinter) isESObjectPossiblyAllowed(typeReference *ast.Node) bool {
	parent := typeReference.Parent
	if parent == nil {
		return false
	}
	switch parent.Kind {
	case ast.KindVariableDeclaration, ast.KindPropertyDeclaration, ast.KindParameter,
		ast.KindFunctionType, ast.KindPropertySignature, ast.KindArrayType, ast.KindNewExpression:
		return true
	case ast.KindArrowFunction, ast.KindFunctionDeclaration, ast.KindMethodDeclaration:
		return !l.functionReturnsObjectLiteral(parent, false)
	case ast.KindTypeReference:
		if entityNameToString(parent.AsTypeReferenceNode().TypeName) == "Promise" {
			return parent.Parent == nil || !l.functionReturnsObjectLiteral(parent.Parent, true)
		}
		return true
	case ast.KindAsExpression:
		return !ast.IsObjectLiteralExpression(parent.AsAsExpression().Expression)
	default:
		return false
	}
}

func (l *arkTSLinter) functionReturnsObjectLiteral(node *ast.Node, promise bool) bool {
	if node == nil || (!ast.IsFunctionDeclaration(node) && !ast.IsMethodDeclaration(node) && !ast.IsArrowFunction(node)) || node.Body() == nil || !ast.IsBlock(node.Body()) {
		return false
	}
	if promise && node.ModifierFlags()&ast.ModifierFlagsAsync == 0 {
		return false
	}
	found := false
	var visit func(*ast.Node)
	visit = func(current *ast.Node) {
		if current == nil || found {
			return
		}
		if current != node.Body() && (ast.IsFunctionDeclaration(current) || ast.IsFunctionExpression(current) || ast.IsMethodDeclaration(current) || ast.IsAccessor(current) || ast.IsArrowFunction(current)) {
			return
		}
		if ast.IsReturnStatement(current) && current.Expression() != nil && ast.IsObjectLiteralExpression(current.Expression()) {
			found = true
			return
		}
		current.ForEachChild(func(child *ast.Node) bool {
			visit(child)
			return false
		})
	}
	visit(node.Body())
	return found
}

func (l *arkTSLinter) variableDeclarationTypeNode(node *ast.Node) *ast.Node {
	symbol := l.trueSymbolAtLocation(node)
	if symbol == nil || len(symbol.Declarations) == 0 || !ast.IsVariableDeclaration(symbol.Declarations[0]) {
		return nil
	}
	return symbol.Declarations[0].Type()
}

func (l *arkTSLinter) handleESObjectAssignment(node *ast.Node, declaredType *ast.Node, initializer *ast.Node, property bool) {
	if declaredType == nil || initializer == nil {
		return
	}
	if ast.IsTypeReferenceNode(declaredType) && entityNameToString(declaredType.AsTypeReferenceNode().TypeName) == "ESObject" {
		if ast.IsObjectLiteralExpression(initializer) {
			l.add(node, arkTSESObjectType)
		}
		return
	}
	if property {
		return
	}
	initializerType := l.variableDeclarationTypeNode(initializer)
	if initializerType != nil && ast.IsTypeReferenceNode(initializerType) && entityNameToString(initializerType.AsTypeReferenceNode().TypeName) == "ESObject" {
		l.add(node, arkTSESObjectType)
	}
}

func (l *arkTSLinter) handleEnumMember(node *ast.Node) {
	diagnosticCount := len(l.diagnostics)
	member := node.AsEnumMember()
	if member.Initializer != nil {
		switch member.Initializer.Kind {
		case ast.KindStringLiteral, ast.KindNumericLiteral, ast.KindPrefixUnaryExpression:
		default:
			if l.checker.evaluate(member.Initializer, node).Value == nil {
				l.add(node, arkTSEnumMemberNonConstInit)
			}
		}
	}
	if node.Parent == nil || !ast.IsEnumDeclaration(node.Parent) || len(node.Parent.AsEnumDeclaration().Members.Nodes) == 0 {
		return
	}
	first := node.Parent.AsEnumDeclaration().Members.Nodes[0]
	currentValue := l.checker.GetConstantValue(node)
	firstValue := l.checker.GetConstantValue(first)
	if _, ok := currentValue.(string); ok {
		if _, firstOK := firstValue.(string); firstOK {
			return
		}
	}
	if isArkTSNumericConstant(currentValue) && isArkTSNumericConstant(firstValue) {
		return
	}
	if len(l.diagnostics) == diagnosticCount && l.checker.getTypeOfNode(first) != l.checker.getTypeOfNode(node) {
		l.add(node, arkTSEnumMemberNonConstInit)
	}
}

func isArkTSNumericConstant(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, jsnum.Number:
		return true
	default:
		return false
	}
}

func (l *arkTSLinter) handleTypeReference(node *ast.Node) {
	typeReference := node.AsTypeReferenceNode()
	name := entityNameToString(typeReference.TypeName)
	if name == "ESObject" {
		if !l.isESObjectPossiblyAllowed(node) {
			l.add(node, arkTSESObjectType)
		}
		return
	}
	for _, unsupported := range []string{
		"Awaited", "Pick", "Omit", "Exclude", "Extract", "NonNullable", "Parameters", "ConstructorParameters", "ReturnType", "InstanceType",
		"ThisParameterType", "OmitThisParameter", "ThisType", "Uppercase", "Lowercase", "Capitalize", "Uncapitalize",
	} {
		if name == unsupported {
			l.add(node, arkTSUtilityType)
			return
		}
	}
	if name == "Partial" && typeReference.TypeArguments != nil && len(typeReference.TypeArguments.Nodes) == 1 {
		t := l.checker.getTypeFromTypeNode(typeReference.TypeArguments.Nodes[0])
		if t != nil && t.objectFlags&ObjectFlagsClassOrInterface == 0 {
			l.add(node, arkTSUtilityType)
		}
	}
	// getTypeAtLocationForLinter(typeRef.typeName) in the OH checker yields the
	// referenced instance type. TSGO's expression query yields the constructor
	// side for a class name, so query the complete TypeReferenceNode instead.
	typeNameType := l.checker.getTypeFromTypeNode(node)
	if l.isSendableClassOrInterface(typeNameType) && typeReference.TypeArguments != nil {
		for _, argument := range typeReference.TypeArguments.Nodes {
			if !l.isSendableTypeNode(argument, false, make(map[*ast.Symbol]struct{})) {
				l.add(argument, arkTSSendableGenericTypes)
			}
		}
	}
}

func (l *arkTSLinter) handleCallExpression(node *ast.Node) {
	call := node.AsCallExpression()
	calleeSymbol := l.trueSymbolAtLocation(call.Expression)
	if call.Expression.Kind == ast.KindImportKeyword && len(call.Arguments.Nodes) > 1 && ast.IsObjectLiteralExpression(call.Arguments.Nodes[1]) {
		for _, property := range call.Arguments.Nodes[1].AsObjectLiteralExpression().Properties.Nodes {
			if property.Name() != nil && property.Name().Text() == "assert" {
				l.add(property, arkTSImportAssertion)
			}
		}
	}
	if ast.IsIdentifier(call.Expression) && call.Expression.Text() == "require" && ast.IsVariableDeclaration(node.Parent) {
		requireType := l.checker.getTypeOfNode(call.Expression)
		if requireType != nil && requireType.symbol != nil && requireType.symbol.Flags&ast.SymbolFlagsInterface != 0 && requireType.symbol.Name == "NodeRequire" {
			l.add(node.Parent, arkTSImportAssignment)
		}
	}
	if calleeSymbol != nil {
		qualifiedName := l.checker.GetFullyQualifiedName(calleeSymbol)
		switch qualifiedName {
		case "Function.apply", "Function.call", "CallableFunction.apply", "CallableFunction.call":
			l.add(node, arkTSFunctionApplyCall)
		case "Function.bind", "CallableFunction.bind":
			l.add(node, arkTSFunctionBind)
		}
		l.handleStdlibAPICall(node, calleeSymbol)
	}
	signature := l.checker.getResolvedSignature(node, nil, CheckModeNormal)
	if signature != nil && !l.isLibrarySymbol(calleeSymbol) {
		l.handleGenericCallWithSignature(node, signature)
		l.handleCallArgumentAssignments(node, signature)
	}
	l.handleTaskpoolCall(node, calleeSymbol)
	l.filterLibraryCallDiagnostics(node, calleeSymbol)
}

func (l *arkTSLinter) filterLibraryCallDiagnostics(node *ast.Node, calleeSymbol *ast.Symbol) {
	isLibraryCall := l.isLibraryType(l.checker.getTypeOfNode(node.Expression()))
	isOhModulesETS := false
	if calleeSymbol != nil && len(calleeSymbol.Declarations) > 0 {
		file := ast.GetSourceFileOfNode(calleeSymbol.Declarations[0])
		isOhModulesETS = file != nil && file.ScriptKind == core.ScriptKindETS && strings.Contains(strings.ToLower(tspath.NormalizePath(file.FileName())), "/oh_modules/")
	}
	allowOHModulesFallback := l.allowOHModulesFallback()
	hasFiltered := false
	deleted := make(map[*ast.Diagnostic]struct{})
	for _, diagnostic := range l.strictDiagnostics {
		if l.strictDiagnosticErrorType(diagnostic) == arkTSStrictPossiblyUndefined {
			continue
		}
		if l.filterLibraryCallDiagnostic(node, diagnostic, isLibraryCall, hasFiltered, isOhModulesETS, allowOHModulesFallback) {
			hasFiltered = false
			deleted[diagnostic] = struct{}{}
			continue
		}
	}
	for _, diagnostic := range l.strictDiagnostics {
		if l.strictDiagnosticErrorType(diagnostic) != arkTSStrictPossiblyUndefined {
			continue
		}
		if l.filterLibraryCallDiagnostic(node, diagnostic, isLibraryCall, hasFiltered, isOhModulesETS, allowOHModulesFallback) {
			hasFiltered = false
			deleted[diagnostic] = struct{}{}
			continue
		}
	}
	l.strictDiagnostics = slices.DeleteFunc(l.strictDiagnostics, func(diagnostic *ast.Diagnostic) bool {
		_, exists := deleted[diagnostic]
		return exists
	})
}

func (l *arkTSLinter) filterLibraryCallDiagnostic(
	node *ast.Node,
	diagnostic *ast.Diagnostic,
	isLibraryCall bool,
	hasFiltered bool,
	isOhModulesETS bool,
	allowOHModulesFallback bool,
) bool {
	errorType := l.strictDiagnosticErrorType(diagnostic)
	if errorType == arkTSStrictNoError || diagnostic.Category() == diagnostics.CategoryWarning || !l.isFilterableStrictErrorType(errorType, isLibraryCall, hasFiltered) || !l.isValidLibraryCallDiagnosticRange(node, diagnostic) {
		return false
	}
	if allowOHModulesFallback && isOhModulesETS && errorType != arkTSStrictUnknown {
		diagnostic.SetCategory(diagnostics.CategoryWarning)
		return false
	}
	return true
}

func (l *arkTSLinter) strictDiagnosticErrorType(diagnostic *ast.Diagnostic) arkTSStrictErrorType {
	if diagnostic == nil {
		return arkTSStrictNoError
	}
	sourceType := ""
	if args := diagnostic.MessageArgs(); len(args) > 0 {
		sourceType = args[0]
	} else {
		sourceType = diagnostic.MessageText()
	}
	switch diagnostic.Code() {
	case 2322:
		if arkTSContainsTypeWord(sourceType, "unknown") {
			if l.checker.compilerOptions.StrictCheckerOnly.IsTrue() && !diagnostic.FilterFlag() {
				return arkTSStrictNoError
			}
			return arkTSStrictUnknown
		}
		if arkTSContainsTypeWord(sourceType, "null") || arkTSContainsTypeWord(sourceType, "undefined") {
			return arkTSStrictNull
		}
	case 2345:
		if arkTSContainsTypeWord(sourceType, "null") || arkTSContainsTypeWord(sourceType, "undefined") {
			return arkTSStrictNull
		}
	case 2532:
		return arkTSStrictPossiblyUndefined
	}
	for _, child := range diagnostic.MessageChain() {
		if result := l.strictDiagnosticErrorType(child); result != arkTSStrictNoError {
			return result
		}
	}
	return arkTSStrictNoError
}

func arkTSContainsTypeWord(text string, word string) bool {
	for offset := 0; ; {
		index := strings.Index(text[offset:], word)
		if index < 0 {
			return false
		}
		index += offset
		before := index == 0 || !arkTSIdentifierByte(text[index-1])
		afterIndex := index + len(word)
		after := afterIndex == len(text) || !arkTSIdentifierByte(text[afterIndex])
		if before && after {
			return true
		}
		offset = index + len(word)
	}
}

func arkTSIdentifierByte(value byte) bool {
	return value == '_' || value == '$' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func (l *arkTSLinter) isFilterableStrictErrorType(errorType arkTSStrictErrorType, isLibraryCall bool, hasFiltered bool) bool {
	switch errorType {
	case arkTSStrictUnknown:
		return true
	case arkTSStrictNull:
		return isLibraryCall
	case arkTSStrictPossiblyUndefined:
		return isLibraryCall && hasFiltered
	default:
		return false
	}
}

func (l *arkTSLinter) isValidLibraryCallDiagnosticRange(node *ast.Node, diagnostic *ast.Diagnostic) bool {
	start := scanner.GetTokenPosOfNode(node, l.file, false)
	fullCall := diagnostic.Len() != 0 && diagnostic.Pos()+diagnostic.Len() == node.End() && diagnostic.Pos() >= start
	validArgument := l.isValidLibraryCallArgumentPosition(node, diagnostic.Pos())
	switch diagnostic.Code() {
	case 2322, 2345:
		return validArgument
	case 2769:
		return fullCall || validArgument
	case 2532:
		return fullCall
	default:
		return false
	}
}

func (l *arkTSLinter) isValidLibraryCallArgumentPosition(node *ast.Node, position int) bool {
	arguments := node.ArgumentList()
	if arguments == nil || position < arguments.Pos() || position >= arguments.End() {
		return false
	}
	for _, argument := range arguments.Nodes {
		var begin int
		var end int
		switch {
		case ast.IsArrowFunction(argument):
			body := argument.Body()
			begin, end = body.Pos(), body.End()
		case ast.IsCallExpression(argument):
			nested := argument.ArgumentList()
			if nested == nil {
				continue
			}
			begin, end = nested.Pos(), nested.End()
		default:
			continue
		}
		if position >= begin && position < end {
			return false
		}
	}
	return true
}

func (l *arkTSLinter) allowOHModulesFallback() bool {
	paths := l.checker.compilerOptions.DisableStrictCheckPaths
	if paths == nil {
		paths = []string{"node_modules", "oh_modules", "build", ".preview"}
	}
	for _, path := range paths {
		if path == "oh_modules" && l.checker.compilerOptions.EnableStrictCheckOHModule != core.TSTrue {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) handleStdlibAPICall(node *ast.Node, symbol *ast.Symbol) {
	name := symbol.Name
	parentName := ""
	if symbol.Parent != nil {
		parentName = symbol.Parent.Name
	}
	if parentName == "" {
		if name == "eval" {
			l.add(node, arkTSLimitedStdLibAPI)
		} else if name == "Symbol" || name == "SymbolConstructor" {
			l.add(node, arkTSSymbolType)
		}
		return
	}
	limitedObject := map[string]struct{}{
		"__proto__": {}, "__defineGetter__": {}, "__defineSetter__": {}, "__lookupGetter__": {}, "__lookupSetter__": {},
		"assign": {}, "create": {}, "defineProperties": {}, "defineProperty": {}, "freeze": {}, "fromEntries": {},
		"getOwnPropertyDescriptor": {}, "getOwnPropertyDescriptors": {}, "getOwnPropertySymbols": {}, "getPrototypeOf": {},
		"hasOwnProperty": {}, "is": {}, "isExtensible": {}, "isFrozen": {}, "isPrototypeOf": {}, "isSealed": {},
		"preventExtensions": {}, "propertyIsEnumerable": {}, "seal": {}, "setPrototypeOf": {},
	}
	limitedReflect := map[string]struct{}{
		"apply": {}, "construct": {}, "defineProperty": {}, "deleteProperty": {}, "getOwnPropertyDescriptor": {},
		"getPrototypeOf": {}, "isExtensible": {}, "preventExtensions": {}, "setPrototypeOf": {},
	}
	limitedProxy := map[string]struct{}{
		"apply": {}, "construct": {}, "defineProperty": {}, "deleteProperty": {}, "get": {}, "getOwnPropertyDescriptor": {},
		"getPrototypeOf": {}, "has": {}, "isExtensible": {}, "ownKeys": {}, "preventExtensions": {}, "set": {}, "setPrototypeOf": {},
	}
	switch parentName {
	case "Object", "ObjectConstructor":
		if _, exists := limitedObject[name]; exists {
			l.add(node, arkTSLimitedStdLibAPI)
		}
	case "Reflect":
		if _, exists := limitedReflect[name]; exists {
			l.add(node, arkTSLimitedStdLibAPI)
		}
	case "ProxyHandler":
		if _, exists := limitedProxy[name]; exists {
			l.add(node, arkTSLimitedStdLibAPI)
		}
	case "Symbol", "SymbolConstructor":
		l.add(node, arkTSSymbolType)
	}
}

func (l *arkTSLinter) isSymbolAPI(symbol *ast.Symbol) bool {
	if symbol == nil {
		return false
	}
	if symbol.Parent != nil && (symbol.Parent.Name == "Symbol" || symbol.Parent.Name == "SymbolConstructor") {
		return true
	}
	return symbol.Parent == nil && (symbol.Name == "Symbol" || symbol.Name == "SymbolConstructor")
}

func (l *arkTSLinter) handleGenericCall(node *ast.Node) {
	signature := l.checker.getResolvedSignature(node, nil, CheckModeNormal)
	if signature != nil {
		l.handleGenericCallWithSignature(node, signature)
		l.handleCallArgumentAssignments(node, signature)
	}
	if ast.IsNewExpression(node) && l.isSendableClassOrInterface(l.checker.getTypeOfNode(node)) {
		for _, argument := range node.TypeArguments() {
			if !l.isSendableTypeNode(argument, false, make(map[*ast.Symbol]struct{})) {
				l.add(argument, arkTSSendableGenericTypes)
			}
		}
	}
}

func (l *arkTSLinter) handleGenericCallWithSignature(node *ast.Node, signature *Signature) {
	if signature == nil || signature.mapper == nil {
		return
	}
	parameters := signature.typeParameters
	if signature.target != nil {
		parameters = signature.target.typeParameters
	}
	inferred := l.checker.instantiateTypes(parameters, signature.mapper)
	start := len(node.TypeArguments())
	for index := start; index < len(inferred); index++ {
		if inferred[index].flags&TypeFlagsUnknown != 0 {
			l.add(node, arkTSGenericCallNoTypeArgs)
			return
		}
	}
}

func (l *arkTSLinter) handleSpread(node *ast.Node) {
	if ast.IsSpreadElement(node) && (ast.IsCallLikeExpression(node.Parent) || ast.IsArrayLiteralExpression(node.Parent)) {
		t := l.typeOrConstraint(node.Expression())
		// OH TypeChecker.isArrayType excludes tuples. TSGO's tuple target uses
		// the Array symbol, so preserve the upstream predicate explicitly.
		if t != nil && !t.IsTupleType() && l.isOrDerivedFrom(t, l.isSpreadArrayType) {
			return
		}
	}
	l.add(node, arkTSSpreadOperator)
}

func (l *arkTSLinter) handleComputedPropertyName(node *ast.Node) {
	declaration := node.Parent
	if declaration != nil {
		declaration = declaration.Parent
	}
	if declaration != nil && (ast.IsClassDeclaration(declaration) && l.hasDecorator(declaration, "Sendable") || ast.IsInterfaceDeclaration(declaration) && l.isSendableClassOrInterface(l.checker.getTypeOfNode(declaration))) {
		if l.isSymbolIteratorComputedName(node) && l.isArkTSCollectionsDeclaration([]*ast.Node{declaration}) {
			return
		}
		l.add(node, arkTSSendableComputedPropName)
		return
	}
	if l.isValidComputedPropertyName(node, false) {
		return
	}
	l.add(node, arkTSComputedPropertyName)
}

func (l *arkTSLinter) isSymbolIteratorComputedName(node *ast.Node) bool {
	if node == nil || !ast.IsComputedPropertyName(node) {
		return false
	}
	expression := node.AsComputedPropertyName().Expression
	if !ast.IsPropertyAccessExpression(expression) || expression.Name().Text() != "iterator" {
		return false
	}
	symbol := l.trueSymbolAtLocation(expression)
	return symbol != nil && symbol.Name == "iterator" && symbol.Parent != nil && (symbol.Parent.Name == "Symbol" || symbol.Parent.Name == "SymbolConstructor")
}

func (l *arkTSLinter) handleThrow(node *ast.Node) {
	t := l.checker.getTypeOfNode(node.Expression())
	if t == nil || t.objectFlags&ObjectFlagsClassOrInterface == 0 || !l.isOrDerivedFromGlobal(t, "Error") {
		l.add(node, arkTSThrowStatement)
	}
}

func (l *arkTSLinter) isOrDerivedFromGlobal(t *Type, name string) bool {
	seen := make(map[*Type]struct{})
	var visit func(*Type) bool
	visit = func(current *Type) bool {
		if current == nil {
			return false
		}
		if _, exists := seen[current]; exists {
			return false
		}
		seen[current] = struct{}{}
		if current.symbol != nil && current.symbol.Name == name {
			return true
		}
		for _, base := range l.checker.getBaseTypes(current) {
			if visit(base) {
				return true
			}
		}
		return false
	}
	return visit(t)
}

func (l *arkTSLinter) isLibraryType(t *Type) bool {
	if t == nil {
		return false
	}
	t = l.checker.GetNonNullableType(t)
	if t.IsUnion() {
		for _, component := range t.Types() {
			if !l.isLibraryType(component) {
				return false
			}
		}
		return true
	}
	symbol := t.symbol
	if t.alias != nil && t.alias.Symbol() != nil {
		symbol = t.alias.Symbol()
	}
	return l.isLibrarySymbol(symbol)
}

func (l *arkTSLinter) isLibrarySymbol(symbol *ast.Symbol) bool {
	if symbol == nil || len(symbol.Declarations) == 0 {
		return false
	}
	file := ast.GetSourceFileOfNode(symbol.Declarations[0])
	if file == nil || l.checker.program.IsSourceFileDefaultLibrary(file.Path()) {
		return false
	}
	name := strings.ToLower(tspath.NormalizePath(file.FileName()))
	thirdParty := strings.Contains(name, "/node_modules/") || strings.Contains(name, "/oh_modules/") ||
		strings.Contains(name, "/build/") || strings.Contains(name, "/.preview/") || tspath.GetBaseFileName(name) == "hvigorfile.ts"
	// Utils.ts::isLibrarySymbol treats only first-party ETS source as static in
	// production. Ordinary TS, declaration files, and third-party ETS obey the
	// interop/library rules. The upstream-only testMode exception for .ts files
	// is intentionally not part of the production compiler API.
	return file.ScriptKind != core.ScriptKindETS || thirdParty
}

func (l *arkTSLinter) isStaticSourceType(t *Type) bool {
	if l.checker.compilerOptions.MixCompile != core.TSTrue || t == nil || t.symbol == nil || len(t.symbol.Declarations) == 0 {
		return false
	}
	file := ast.GetSourceFileOfNode(t.symbol.Declarations[0])
	return file != nil && file.ScriptKind == core.ScriptKindETS
}

func (l *arkTSLinter) isDynamicLiteralInitializer(node *ast.Node) bool {
	if node == nil || !ast.IsObjectLiteralExpression(node) && !ast.IsArrayLiteralExpression(node) {
		return false
	}
	current := node
	for ast.IsObjectLiteralExpression(current) || ast.IsArrayLiteralExpression(current) {
		contextualType := l.checker.getContextualType(current, ContextFlagsNone)
		if contextualType != nil && contextualType.objectFlags&ObjectFlagsAnonymous == 0 && l.isLibraryType(contextualType) {
			return true
		}
		current = current.Parent
		if ast.IsPropertyAssignment(current) {
			current = current.Parent
		}
	}
	if ast.IsCallExpression(current) {
		calleeType := l.checker.getTypeOfNode(current.Expression())
		if IsTypeAny(calleeType) || calleeType != nil && l.isLibrarySymbol(calleeType.symbol) {
			return true
		}
		if ast.IsPropertyAccessExpression(current.Expression()) {
			symbol := l.trueSymbolAtLocation(current.Expression().Expression())
			return l.isLibrarySymbol(symbol)
		}
	}
	if ast.IsBinaryExpression(current) && ast.IsPropertyAccessExpression(current.AsBinaryExpression().Left) {
		return l.isLibraryType(l.checker.getTypeOfNode(current.AsBinaryExpression().Left.Expression()))
	}
	return false
}

func (l *arkTSLinter) trueSymbolAtLocation(node *ast.Node) *ast.Symbol {
	symbol := l.checker.getSymbolAtLocation(node, false)
	if symbol != nil && symbol.Flags&ast.SymbolFlagsAlias != 0 {
		return l.checker.resolveAlias(symbol)
	}
	return symbol
}

func (l *arkTSLinter) isValidComputedPropertyName(node *ast.Node, record bool) bool {
	expression := node.AsComputedPropertyName().Expression
	if ast.IsStringLiteralLike(expression) {
		return true
	}
	symbol := l.trueSymbolAtLocation(expression)
	if symbol != nil && symbol.Flags&ast.SymbolFlagsEnumMember != 0 {
		t := l.checker.getTypeOfNode(expression)
		return t != nil && t.flags&(TypeFlagsEnumLike|TypeFlagsStringLiteral) == (TypeFlagsEnumLiteral|TypeFlagsStringLiteral)
	}
	return !record && symbol != nil && symbol.Name == "iterator" && symbol.Parent != nil && (symbol.Parent.Name == "Symbol" || symbol.Parent.Name == "SymbolConstructor")
}

func (l *arkTSLinter) isLiteralInitializerForSendableType(contextualType *Type, node *ast.Node) bool {
	if !contextualType.IsUnion() {
		return l.isSendableClassOrInterface(contextualType)
	}
	literalType := l.checker.getTypeOfNode(node)
	matched := false
	for _, component := range contextualType.Types() {
		if l.checker.isTypeAssignableTo(literalType, component) {
			matched = true
			if !l.isSendableClassOrInterface(component) {
				return false
			}
		}
	}
	return matched
}

func (l *arkTSLinter) isSendableClassOrInterface(t *Type) bool {
	if t == nil {
		return false
	}
	if t.flags&TypeFlagsObject != 0 && t.objectFlags&ObjectFlagsReference != 0 && t.AsObjectType().target != nil {
		t = t.AsObjectType().target
	}
	if t.IsClass() && t.symbol != nil && len(t.symbol.Declarations) > 0 && l.hasDecorator(t.symbol.Declarations[0], "Sendable") {
		return true
	}
	seen := make(map[*Type]struct{})
	var visit func(*Type) bool
	visit = func(current *Type) bool {
		if current == nil {
			return false
		}
		if _, exists := seen[current]; exists {
			return false
		}
		seen[current] = struct{}{}
		if current.symbol != nil && len(current.symbol.Declarations) > 0 {
			declaration := current.symbol.Declarations[0]
			file := ast.GetSourceFileOfNode(declaration)
			if ast.IsInterfaceDeclaration(declaration) && declaration.Name().Text() == "ISendable" && declaration.Parent != nil && ast.IsModuleBlock(declaration.Parent) && declaration.Parent.Parent != nil && declaration.Parent.Parent.Name().Text() == "lang" && file != nil && strings.EqualFold(tspath.GetBaseFileName(file.FileName()), "@arkts.lang.d.ets") {
				return true
			}
		}
		for _, base := range l.checker.getBaseTypes(current) {
			if visit(base) {
				return true
			}
		}
		return false
	}
	return visit(t)
}

func (l *arkTSLinter) isISendableInterface(t *Type) bool {
	if t == nil {
		return false
	}
	symbol := t.symbol
	if t.alias != nil && t.alias.Symbol() != nil {
		symbol = t.alias.Symbol()
	}
	if symbol == nil || len(symbol.Declarations) == 0 {
		return false
	}
	declaration := symbol.Declarations[0]
	file := ast.GetSourceFileOfNode(declaration)
	return ast.IsInterfaceDeclaration(declaration) && declaration.Name() != nil && declaration.Name().Text() == "ISendable" &&
		declaration.Parent != nil && ast.IsModuleBlock(declaration.Parent) && declaration.Parent.Parent != nil && declaration.Parent.Parent.Name() != nil && declaration.Parent.Parent.Name().Text() == "lang" &&
		file != nil && strings.EqualFold(tspath.GetBaseFileName(file.FileName()), "@arkts.lang.d.ets")
}

func (l *arkTSLinter) isSendableType(t *Type) bool {
	if t == nil {
		return false
	}
	if t.flags&(TypeFlagsBoolean|TypeFlagsNumber|TypeFlagsString|TypeFlagsBigInt|TypeFlagsNull|TypeFlagsUndefined|TypeFlagsTypeParameter) != 0 {
		return true
	}
	return l.isSendableTypeAlias(t) || l.isSendableFunction(t) || l.isSendableClassOrInterface(t)
}

func (l *arkTSLinter) isSendableTypeAlias(t *Type) bool {
	declaration := l.originalTypeAliasDeclaration(t, make(map[*ast.Symbol]struct{}))
	return declaration != nil && l.hasDecorator(declaration, "Sendable")
}

func (l *arkTSLinter) originalTypeAliasDeclaration(t *Type, seen map[*ast.Symbol]struct{}) *ast.Node {
	if t == nil || t.alias == nil || t.alias.Symbol() == nil {
		return nil
	}
	symbol := t.alias.Symbol()
	if _, exists := seen[symbol]; exists {
		return nil
	}
	seen[symbol] = struct{}{}
	if len(symbol.Declarations) == 0 || !ast.IsTypeAliasDeclaration(symbol.Declarations[0]) {
		return nil
	}
	declaration := symbol.Declarations[0]
	typeNode := declaration.Type()
	if ast.IsTypeReferenceNode(typeNode) {
		target := l.checker.getTypeOfNode(typeNode.AsTypeReferenceNode().TypeName)
		if target != nil && target.alias != nil && target.alias.Symbol() != nil && target.alias.Symbol().Flags&ast.SymbolFlagsTypeAlias != 0 {
			if original := l.originalTypeAliasDeclaration(target, seen); original != nil {
				return original
			}
		}
	}
	return declaration
}

func (l *arkTSLinter) isSendableFunction(t *Type) bool {
	signatures := l.checker.getSignaturesOfType(t, SignatureKindCall)
	if len(signatures) == 0 || signatures[0].declaration == nil || !ast.IsFunctionDeclaration(signatures[0].declaration) {
		return false
	}
	declaration := signatures[0].declaration
	symbol := declaration.Symbol()
	if symbol == nil {
		symbol = l.checker.getSymbolAtLocation(declaration.Name(), false)
	}
	if symbol == nil {
		return l.hasDecorator(declaration, "Sendable")
	}
	for _, overload := range symbol.Declarations {
		if ast.IsFunctionDeclaration(overload) && l.hasDecorator(overload, "Sendable") {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) hasSendableTypeAlias(t *Type) bool {
	if t == nil {
		return false
	}
	if t.IsUnion() {
		for _, component := range t.Types() {
			if l.hasSendableTypeAlias(component) {
				return true
			}
		}
		return false
	}
	return l.isSendableTypeAlias(t)
}

func (l *arkTSLinter) isWrongSendableFunctionAssignment(leftType *Type, rightType *Type) bool {
	leftType = l.checker.GetNonNullableType(leftType)
	rightType = l.checker.GetNonNullableType(rightType)
	if !l.hasSendableTypeAlias(leftType) || rightType == nil {
		return false
	}
	if rightType.IsUnion() {
		for _, component := range rightType.Types() {
			if l.isInvalidSendableFunctionAssignment(component) {
				return true
			}
		}
		return false
	}
	return l.isInvalidSendableFunctionAssignment(rightType)
}

func (l *arkTSLinter) isInvalidSendableFunctionAssignment(t *Type) bool {
	if t == nil {
		return false
	}
	if t.alias != nil && t.alias.Symbol() != nil {
		declaration := l.originalTypeAliasDeclaration(t, make(map[*ast.Symbol]struct{}))
		return declaration != nil && ast.IsFunctionTypeNode(declaration.Type()) && !l.hasDecorator(declaration, "Sendable")
	}
	return len(l.checker.getSignaturesOfType(t, SignatureKindCall)) > 0 && !l.isSendableFunction(t)
}

func (l *arkTSLinter) handleSendableAsExpression(node *ast.Node, targetType *Type, expressionType *Type) {
	if targetType == nil || expressionType == nil {
		return
	}
	if !l.isSendableClassOrInterface(expressionType) && !l.isObjectType(expressionType) && !IsTypeAny(expressionType) && l.isSendableClassOrInterface(targetType) {
		l.add(node, arkTSSendableAsExpression)
	}
	if l.isWrongSendableFunctionAssignment(targetType, expressionType) {
		l.add(node, arkTSSendableFunctionAsExpression)
	}
}

func (l *arkTSLinter) isObjectType(t *Type) bool {
	return t != nil && (t.flags&TypeFlagsNonPrimitive != 0 || t.symbol != nil && t.objectFlags&ObjectFlagsClassOrInterface != 0 && t.symbol.Name == "Object")
}

func (l *arkTSLinter) hasDecorator(node *ast.Node, name string) bool {
	for _, modifier := range node.ModifierNodes() {
		if !ast.IsDecorator(modifier) {
			continue
		}
		expression := modifier.Expression()
		if ast.IsCallExpression(expression) {
			expression = expression.Expression()
		}
		if ast.IsIdentifier(expression) && expression.Text() == name {
			return true
		}
	}
	return false
}

func (l *arkTSLinter) decoratorName(node *ast.Node) string {
	if node == nil || !ast.IsDecorator(node) {
		return ""
	}
	expression := node.Expression()
	if ast.IsCallExpression(expression) {
		expression = expression.Expression()
	}
	if ast.IsIdentifier(expression) {
		return expression.Text()
	}
	return ""
}

func (l *arkTSLinter) handleDecorator(node *ast.Node) {
	if l.decoratorName(node) != "Sendable" {
		return
	}
	if node.Parent == nil || ast.IsStructDeclaration(node.Parent) || node.Parent.Kind != ast.KindClassDeclaration && node.Parent.Kind != ast.KindFunctionDeclaration && node.Parent.Kind != ast.KindTypeAliasDeclaration {
		l.add(node, arkTSSendableDecoratorLimited)
	}
}

func (l *arkTSLinter) handleExpressionWithTypeArguments(node *ast.Node) {
	symbol := l.trueSymbolAtLocation(node.Expression())
	if symbol == nil || len(symbol.Declarations) == 0 {
		return
	}
	declaration := symbol.Declarations[0]
	if ast.IsTypeAliasDeclaration(declaration) && declaration.Name() != nil && declaration.Name().Text() == "ESObject" && declaration.Type() != nil && declaration.Type().Kind == ast.KindAnyKeyword {
		l.add(node, arkTSESObjectType)
	}
}

func (l *arkTSLinter) isTaskpoolAPI(symbol *ast.Symbol, node *ast.Node) bool {
	if symbol == nil || len(symbol.Declarations) == 0 || !ast.GetSourceFileOfNode(symbol.Declarations[0]).IsDeclarationFile || !ast.IsPropertyAccessExpression(node) {
		return false
	}
	allowed := symbol.Name == "Task" || symbol.Name == "LongTask" || symbol.Name == "GenericsTask" || symbol.Name == "execute" || symbol.Name == "addTask"
	if !allowed {
		return false
	}
	baseSymbol := l.trueSymbolAtLocation(node.Expression())
	if baseSymbol == nil {
		return false
	}
	if baseSymbol.Name == "taskpool" || baseSymbol.Name == "TaskGroup" {
		return true
	}
	return l.checker.TypeToString(l.checker.getTypeOfSymbol(baseSymbol)) == "TaskGroup"
}

func (l *arkTSLinter) handleTaskpoolCall(node *ast.Node, symbol *ast.Symbol) {
	if !l.isTaskpoolAPI(symbol, node.Expression()) || len(node.Arguments()) == 0 {
		return
	}
	argument := node.Arguments()[0]
	argumentType := l.checker.getTypeOfNode(argument)
	argumentSymbol := argumentType.symbol
	if argumentSymbol == nil || argumentSymbol.Name == "Task" || argumentSymbol.Name == "LongTask" || argumentSymbol.Name == "GenericsTask" || argumentSymbol.Name == "TaskGroup" {
		return
	}
	if l.invalidTaskpoolFunction(argument, argumentType, argumentSymbol) {
		l.add(argument, arkTSTaskpoolFunctionArg)
	}
}

func (l *arkTSLinter) handleTaskpoolNew(node *ast.Node) {
	arguments := node.Arguments()
	for index := 0; index < min(len(arguments), 2); index++ {
		argument := arguments[index]
		argumentType := l.checker.getTypeOfNode(argument)
		if argumentType != nil && argumentType.flags&TypeFlagsStringLike != 0 {
			continue
		}
		var argumentSymbol *ast.Symbol
		if argumentType != nil {
			argumentSymbol = argumentType.symbol
		}
		if l.invalidTaskpoolFunction(argument, argumentType, argumentSymbol) {
			l.add(argument, arkTSTaskpoolFunctionArg)
		}
		break
	}
}

func (l *arkTSLinter) invalidTaskpoolFunction(argument *ast.Node, argumentType *Type, argumentSymbol *ast.Symbol) bool {
	declarationSymbol := argumentSymbol != nil && len(argumentSymbol.Declarations) > 0 && ast.GetSourceFileOfNode(argumentSymbol.Declarations[0]).IsDeclarationFile
	trueSymbol := l.trueSymbolAtLocation(argument)
	var declaration *ast.Node
	if trueSymbol != nil && len(trueSymbol.Declarations) > 0 {
		declaration = trueSymbol.Declarations[0]
	}
	if declarationSymbol {
		return declaration != nil && ast.IsPropertySignatureDeclaration(declaration) && declaration.Name() != nil && declaration.Name().Text() == "constructor" && declaration.Parent != nil && ast.IsInterfaceDeclaration(declaration.Parent) && declaration.Parent.Name().Text() == "Object" && strings.EqualFold(tspath.GetBaseFileName(ast.GetSourceFileOfNode(declaration).FileName()), "lib.es5.d.ts")
	}
	if argumentSymbol != nil && argumentSymbol.Flags&ast.SymbolFlagsFunction != 0 {
		return !l.isConcurrentFunction(argumentType)
	}
	return declaration != nil && (ast.IsMethodDeclaration(declaration) || ast.IsClassDeclaration(declaration))
}

func (l *arkTSLinter) isConcurrentFunction(t *Type) bool {
	if t == nil {
		return false
	}
	signatures := l.checker.getSignaturesOfType(t, SignatureKindCall)
	if len(signatures) == 0 || signatures[0].declaration == nil || !ast.IsFunctionDeclaration(signatures[0].declaration) {
		return false
	}
	declaration := signatures[0].declaration
	symbol := declaration.Symbol()
	if symbol != nil {
		for _, overload := range symbol.Declarations {
			if ast.IsFunctionDeclaration(overload) && l.hasDecorator(overload, "Concurrent") {
				return true
			}
		}
	}
	body := declaration.Body()
	if body == nil || !ast.IsBlock(body) || len(body.Statements()) == 0 {
		return false
	}
	first := body.Statements()[0]
	return ast.IsExpressionStatement(first) && ast.IsStringLiteral(first.Expression()) && strings.TrimSpace(first.Expression().Text()) == "use concurrent"
}

func (l *arkTSLinter) inSharedModule() bool {
	for _, statement := range l.file.Statements.Nodes {
		if ast.IsImportDeclaration(statement) {
			continue
		}
		return ast.IsExpressionStatement(statement) && ast.IsStringLiteral(statement.Expression()) && statement.Expression().Text() == "use shared"
	}
	return false
}

func (l *arkTSLinter) handleSharedExportAssignment(node *ast.Node) {
	if !l.inSharedModule() {
		return
	}
	expression := node.AsExportAssignment().Expression
	if !l.isShareableEntity(expression) {
		l.add(expression, arkTSSharedModuleExports)
	}
}

func (l *arkTSLinter) handleSharedExportKeyword(node *ast.Node) {
	declaration := node.Parent
	if !l.inSharedModule() || declaration == nil || declaration.Parent != nil && ast.IsModuleBlock(declaration.Parent) {
		return
	}
	switch declaration.Kind {
	case ast.KindEnumDeclaration:
		symbol := declaration.Symbol()
		if symbol == nil && declaration.Name() != nil {
			symbol = l.trueSymbolAtLocation(declaration.Name())
		}
		if symbol == nil || !isConstEnumSymbol(symbol) {
			l.add(declaration.Name(), arkTSSharedModuleExports)
		}
	case ast.KindInterfaceDeclaration, ast.KindFunctionDeclaration, ast.KindClassDeclaration:
		if !l.isShareableType(l.checker.getTypeOfNode(declaration)) {
			target := declaration.Name()
			if target == nil {
				target = declaration
			}
			l.add(target, arkTSSharedModuleExports)
		}
	case ast.KindVariableStatement:
		for _, variable := range declaration.AsVariableStatement().DeclarationList.AsVariableDeclarationList().Declarations.Nodes {
			if !l.isShareableEntity(variable.Name()) {
				l.add(variable.Name(), arkTSSharedModuleExports)
			}
		}
	case ast.KindTypeAliasDeclaration:
		if !l.isShareableEntity(declaration) {
			l.add(declaration, arkTSSharedModuleExportsWarning)
		}
	default:
		l.add(declaration, arkTSSharedModuleExports)
	}
}

func (l *arkTSLinter) handleSharedExportDeclaration(node *ast.Node) {
	if !l.inSharedModule() || node.Parent != nil && ast.IsModuleBlock(node.Parent) {
		return
	}
	exportDeclaration := node.AsExportDeclaration()
	if exportDeclaration.ExportClause == nil {
		l.add(node, arkTSSharedModuleNoWildcardExport)
		return
	}
	if ast.IsNamespaceExport(exportDeclaration.ExportClause) {
		name := exportDeclaration.ExportClause.AsNamespaceExport().Name()
		if !l.isShareableType(l.checker.getTypeOfNode(name)) {
			l.add(name, arkTSSharedModuleExports)
		}
		return
	}
	for _, specifier := range exportDeclaration.ExportClause.AsNamedExports().Elements.Nodes {
		name := specifier.Name()
		if !l.isShareableEntity(name) {
			l.add(name, arkTSSharedModuleExports)
		}
	}
}

func (l *arkTSLinter) isShareableEntity(node *ast.Node) bool {
	if node == nil {
		return false
	}
	declaration := node
	if !ast.IsDeclaration(node) {
		symbol := l.trueSymbolAtLocation(node)
		if symbol != nil && len(symbol.Declarations) > 0 {
			declaration = symbol.Declarations[0]
		}
	}
	if typeNode := declaration.Type(); typeNode != nil && !ast.IsFunctionLikeDeclaration(declaration) {
		return l.isSendableTypeNode(typeNode, true, make(map[*ast.Symbol]struct{}))
	}
	return l.isShareableType(l.checker.getTypeOfNode(declaration))
}

func (l *arkTSLinter) isShareableType(t *Type) bool {
	if t == nil {
		return false
	}
	symbol := t.symbol
	if symbol != nil && (isConstEnumSymbol(symbol) || symbol.Flags&ast.SymbolFlagsEnumMember != 0 && symbol.Parent != nil && isConstEnumSymbol(symbol.Parent)) {
		return true
	}
	if t.IsUnion() {
		for _, component := range t.Types() {
			if !l.isShareableType(component) {
				return false
			}
		}
		return true
	}
	if t.flags&(TypeFlagsBooleanLiteral|TypeFlagsNumberLiteral|TypeFlagsStringLiteral|TypeFlagsBigIntLiteral) != 0 && t.flags&TypeFlagsEnumLiteral == 0 {
		return true
	}
	return l.isSendableType(t)
}

func (l *arkTSLinter) checkCommentDirectives() {
	for _, pragma := range l.file.Pragmas {
		if pragma.Name == "ts-nocheck" {
			l.addCommentRange(pragma.TextRange, arkTSErrorSuppression)
		}
	}
	for _, directive := range l.file.CommentDirectives {
		l.addCommentRange(directive.Loc, arkTSErrorSuppression)
	}
}

func (l *arkTSLinter) addCommentRange(loc core.TextRange, fault arkTSFault) {
	attribute := arkTSFaultAttributes[fault]
	l.diagnostics = append(l.diagnostics, ast.NewDiagnosticFromText(
		l.file,
		core.NewTextRange(loc.Pos(), loc.End()),
		attribute.code,
		attribute.category,
		attribute.message,
		nil,
		nil,
		false,
		false,
	))
}

func (l *arkTSLinter) visitInterop(node *ast.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case ast.KindImportDeclaration:
		l.handleInteropImportDeclaration(node)
	case ast.KindInterfaceDeclaration, ast.KindClassDeclaration:
		l.handleInteropHeritageClauses(node)
	case ast.KindNewExpression:
		l.handleInteropNewExpression(node)
	case ast.KindObjectLiteralExpression, ast.KindArrayLiteralExpression:
		l.handleInteropLiteralExpression(node)
	case ast.KindAsExpression:
		l.handleInteropAsExpression(node)
	case ast.KindExportDeclaration:
		l.handleInteropExportDeclaration(node)
	case ast.KindExportAssignment:
		l.handleInteropExportAssignment(node)
	}
	node.ForEachChild(func(child *ast.Node) bool {
		l.visitInterop(child)
		return false
	})
}

// InteropTypescriptLinter.ts::checkSendableClassorISendable.
func (l *arkTSLinter) handleInteropImportDeclaration(node *ast.Node) {
	declaration := node.AsImportDeclaration()
	specifier := declaration.ModuleSpecifier
	resolved := l.interopResolvedModule(specifier)
	if resolved == nil {
		return
	}
	baseName := tspath.GetBaseFileName(resolved.ResolvedFileName)
	if strings.HasPrefix(baseName, "@kit.") && strings.HasSuffix(baseName, tspath.ExtensionDts) {
		if l.checker.compilerOptions.EtsLoaderPath == "" || declaration.ImportClause == nil {
			return
		}
		clause := declaration.ImportClause.AsImportClause()
		// InteropTypescriptLinter deliberately skips Kit default imports.
		if clause.Name() != nil || clause.NamedBindings == nil || !ast.IsNamedImports(clause.NamedBindings) {
			return
		}
		l.checkInteropKitBindings(clause.NamedBindings.AsNamedImports().Elements.Nodes, baseName)
		return
	}
	if resolved.Extension != tspath.ExtensionEts && resolved.Extension != tspath.ExtensionDets || l.isInteropImportWhiteListed(resolved) {
		return
	}
	if declaration.ImportClause == nil {
		l.add(node, arkTSNoSideEffectImportETSToTS)
		return
	}
	clause := declaration.ImportClause.AsImportClause()
	if clause.Name() != nil && !l.allowSDKImportSendable(resolved) {
		l.checkInteropImportedEntity(clause.Name())
	}
	if clause.NamedBindings == nil {
		return
	}
	if ast.IsNamespaceImport(clause.NamedBindings) {
		l.add(clause.NamedBindings, arkTSNoNamespaceImportETSToTS)
		return
	}
	if ast.IsNamedImports(clause.NamedBindings) {
		for _, element := range clause.NamedBindings.AsNamedImports().Elements.Nodes {
			l.checkInteropImportedEntity(element.Name())
		}
	}
}

func (l *arkTSLinter) interopResolvedModule(specifier *ast.Node) *module.ResolvedModule {
	if specifier == nil || !ast.IsStringLiteralLike(specifier) {
		return nil
	}
	mode := l.checker.program.GetModeForUsageLocation(l.file, specifier)
	resolved := l.checker.program.GetResolvedModule(l.file, specifier.Text(), mode)
	if resolved == nil || !resolved.IsResolved() {
		return nil
	}
	return resolved
}

func (l *arkTSLinter) isInteropImportWhiteListed(resolved *module.ResolvedModule) bool {
	baseName := tspath.GetBaseFileName(resolved.ResolvedFileName)
	return baseName == "@arkts.lang.d.ets" || baseName == "@arkts.collections.d.ets"
}

func (l *arkTSLinter) allowSDKImportSendable(resolved *module.ResolvedModule) bool {
	sdkRoot, isInSDK := l.interopSDKRoot()
	return isInSDK && strings.HasPrefix(tspath.NormalizePath(resolved.ResolvedFileName), sdkRoot) && strings.Contains(tspath.GetBaseFileName(resolved.ResolvedFileName), "sendable")
}

func (l *arkTSLinter) interopSDKRoot() (string, bool) {
	loaderPath := l.checker.compilerOptions.EtsLoaderPath
	if loaderPath == "" {
		return "", false
	}
	sdkRoot := tspath.NormalizePath(tspath.ResolvePath(loaderPath, "../.."))
	return sdkRoot, strings.HasPrefix(tspath.NormalizePath(l.file.FileName()), sdkRoot)
}

func (l *arkTSLinter) checkInteropImportedEntity(node *ast.Node) {
	if !l.isSendableClassOrInterfaceEntity(node) {
		l.add(node, arkTSNoTSImportETS)
	}
}

func (l *arkTSLinter) isSendableClassOrInterfaceEntity(node *ast.Node) bool {
	symbol := l.trueSymbolAtLocation(node)
	if symbol == nil || len(symbol.Declarations) == 0 {
		return false
	}
	declaration := symbol.Declarations[0]
	if ast.IsClassDeclaration(declaration) {
		return l.hasDecorator(declaration, "Sendable")
	}
	return ast.IsInterfaceDeclaration(declaration) && l.isSendableClassOrInterface(l.checker.getTypeOfNode(declaration))
}

func (l *arkTSLinter) handleInteropHeritageClauses(node *ast.Node) {
	var clauses *ast.NodeList
	if ast.IsClassDeclaration(node) {
		clauses = node.AsClassDeclaration().HeritageClauses
	} else {
		symbol := l.trueSymbolAtLocation(node.Name())
		if symbol == nil || len(symbol.Declarations) == 0 {
			return
		}
		clauses = node.AsInterfaceDeclaration().HeritageClauses
	}
	if clauses == nil {
		return
	}
	for _, clause := range clauses.Nodes {
		for _, heritageType := range clause.AsHeritageClause().Types.Nodes {
			if l.isSendableClassOrInterface(l.reduceReference(l.checker.getTypeOfNode(heritageType))) {
				l.add(heritageType, arkTSSendableTypeInheritance)
			}
		}
	}
}

func (l *arkTSLinter) handleInteropNewExpression(node *ast.Node) {
	if !l.isSendableClassOrInterface(l.checker.getTypeOfNode(node)) {
		return
	}
	for _, argument := range node.TypeArguments() {
		if !l.isSendableTypeNode(argument, false, make(map[*ast.Symbol]struct{})) {
			l.add(argument, arkTSSendableGenericTypes)
		}
	}
}

func (l *arkTSLinter) handleInteropLiteralExpression(node *ast.Node) {
	contextualType := l.checker.getContextualType(node, ContextFlagsNone)
	if contextualType != nil && l.isLiteralInitializerForSendableType(contextualType, node) {
		l.add(node, arkTSSendableObjectInitialization)
	}
}

func (l *arkTSLinter) handleInteropAsExpression(node *ast.Node) {
	assertion := node.AsAsExpression()
	targetType := l.checker.GetNonNullableType(l.checker.getTypeOfNode(assertion.Type))
	expressionType := l.checker.GetNonNullableType(l.checker.getTypeOfNode(assertion.Expression))
	l.handleSendableAsExpression(node, targetType, expressionType)
}

func (l *arkTSLinter) handleInteropExportDeclaration(node *ast.Node) {
	declaration := node.AsExportDeclaration()
	if declaration.ModuleSpecifier != nil && ast.IsStringLiteralLike(declaration.ModuleSpecifier) {
		resolved := l.interopResolvedModule(declaration.ModuleSpecifier)
		if resolved == nil {
			return
		}
		baseName := tspath.GetBaseFileName(resolved.ResolvedFileName)
		if strings.HasPrefix(baseName, "@kit.") && strings.HasSuffix(baseName, tspath.ExtensionDts) {
			if l.checker.compilerOptions.EtsLoaderPath != "" && declaration.ExportClause != nil && ast.IsNamedExports(declaration.ExportClause) {
				l.checkInteropKitBindings(declaration.ExportClause.AsNamedExports().Elements.Nodes, baseName)
			}
			return
		}
		if resolved.Extension == tspath.ExtensionEts || resolved.Extension == tspath.ExtensionDets {
			l.add(declaration.ModuleSpecifier, arkTSNoTSReExportETS)
		}
		return
	}
	if _, isInSDK := l.interopSDKRoot(); !isInSDK || declaration.ExportClause == nil || !ast.IsNamedExports(declaration.ExportClause) {
		return
	}
	for _, specifier := range declaration.ExportClause.AsNamedExports().Elements.Nodes {
		if l.isSendableClassOrInterfaceEntity(specifier.Name()) {
			l.add(specifier.Name(), arkTSSendableTypeExported)
		}
	}
}

func (l *arkTSLinter) handleInteropExportAssignment(node *ast.Node) {
	if _, isInSDK := l.interopSDKRoot(); isInSDK {
		expression := node.AsExportAssignment().Expression
		if l.isSendableClassOrInterfaceEntity(expression) {
			l.add(expression, arkTSSendableTypeExported)
		}
	}
}

func (l *arkTSLinter) checkInteropKitBindings(elements []*ast.Node, kitFileName string) {
	kitInfo, ok := l.interopKitInfo(kitFileName)
	if !ok {
		return
	}
	for _, element := range elements {
		name := element.Name()
		lookupName := name.Text()
		if ast.IsImportSpecifier(element) && element.AsImportSpecifier().PropertyName != nil {
			lookupName = element.AsImportSpecifier().PropertyName.Text()
		} else if ast.IsExportSpecifier(element) && element.AsExportSpecifier().PropertyName != nil {
			lookupName = element.AsExportSpecifier().PropertyName.Text()
		}
		symbol, exists := kitInfo.Symbols[lookupName]
		if !exists || symbol.Source == "" || strings.HasSuffix(symbol.Source, tspath.ExtensionDts) {
			continue
		}
		declarationSymbol := l.trueSymbolAtLocation(name)
		if declarationSymbol == nil || len(declarationSymbol.Declarations) == 0 {
			continue
		}
		declaration := declarationSymbol.Declarations[0]
		if ast.IsModuleDeclaration(declaration) {
			if symbol.Source != "@arkts.collections.d.ets" && symbol.Source != "@arkts.lang.d.ets" {
				l.add(element, arkTSNoTSImportETS)
			}
		} else if !l.isSendableClassOrInterfaceEntity(name) {
			l.add(element, arkTSNoTSImportETS)
		}
	}
}

func (l *arkTSLinter) interopKitInfo(fileName string) (arkTSKitInfo, bool) {
	if l.kitInfos == nil {
		l.kitInfos = make(map[string]arkTSKitInfo)
	}
	if info, exists := l.kitInfos[fileName]; exists {
		return info, true
	}
	loaderPath := l.checker.compilerOptions.EtsLoaderPath
	configRoots := make([]string, 0, 1+len(l.checker.compilerOptions.OhExternalApiPaths))
	configRoots = append(configRoots, tspath.ResolvePath(loaderPath, "../ets-loader/kit_configs"))
	for _, externalAPIPath := range l.checker.compilerOptions.OhExternalApiPaths {
		configRoots = append(configRoots, tspath.ResolvePath(externalAPIPath, "build-tools/ets-loader/kit_configs"))
	}
	configName := strings.TrimSuffix(fileName, tspath.ExtensionDts) + ".json"
	var info arkTSKitInfo
	found := false
	reader, supportsRead := l.checker.program.(interface {
		ReadFile(fileName string) (string, bool)
	})
	if !supportsRead {
		return arkTSKitInfo{}, false
	}
	for _, root := range configRoots {
		contents, ok := reader.ReadFile(tspath.ResolvePath(root, configName))
		if !ok {
			continue
		}
		if json.Unmarshal([]byte(contents), &info) == nil {
			found = true
		}
	}
	if found {
		l.kitInfos[fileName] = info
	}
	return info, found
}
