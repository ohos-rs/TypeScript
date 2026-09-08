package checker

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

const (
	ohRuntimeOS                            = "OpenHarmony"
	ohAvailableFileName                    = "@ohos.annotation.d.ets"
	ohDeviceInfoFileName                   = "@ohos.deviceInfo.d.ts"
	ohApiAvailableName                     = "apiAvailable"
	ohMSFIntegerVersion                    = 26
	ohApiAvailableError                    = "Invalid parameters for apiAvailable."
	ohApiAvailableOpenHarmonyContentError  = "The api version must be a decimal integer between 1 and 25.\n The M.S.F format must meet the following requirements: The value must be in the three decimal format, M must be greater than or equal to 26, and S and F must be decimal integers between 0 and 99."
	ohApiAvailableDistributionContentError = "The api version must be a decimal integer between 1 and 25.\n The M.S.F format must meet the following requirements: The value must be in the three decimal format, M must be decimal integers between 1 and 99, and S and F must be decimal integers between 0 and 99."
	ohAvailableVersionFormatErrorPrefix    = "The runtime OS for the current project is $RUNTIMEOS. The OS version number $VERSION is invalid."
	ohAvailableVersionFormatError          = "The OpenHarmony version must be an integer between 1 and 999,\n and when the OpenHarmony version is greater than or equal to 26, the version number format also supports the M.S.F format."
	ohAvailableOSNameError                 = "The runtime OS for the current project is $RUNTIMEOS. @Available is not supported on the OS: $OSNAME."
	ohAvailableScopeError                  = "Unnecessary. The outer annotation already indicates that the version is greater than or equal to $VERSION."
)

var (
	ohDecimalIntegerPattern          = regexp.MustCompile(`^[+-]?[0-9]+$`)
	ohCanonicalDecimalIntegerPattern = regexp.MustCompile(`^[+-]?(0|[1-9][0-9]*)$`)
	ohOpenHarmonyStringPattern       = regexp.MustCompile(`^[0-9.]+$`)
	ohDistributionStringPattern      = regexp.MustCompile(`^[0-9.()]+$`)
	ohMSFPattern                     = regexp.MustCompile(`^([1-9]\d?)\.(0|[1-9]\d?)\.(0|[1-9]\d?)(?:\((\d+)\))?$`)
	ohAvailableFormatPattern         = regexp.MustCompile(`^(?:[1-9]\d{0,2}|[1-9]\d?\.\d{1,2}\.\d{1,2})$`)
)

type ohParsedVersion struct {
	os            string
	version       string
	formatVersion string
}

type ohSDKCheckResult struct {
	valid    bool
	message  string
	category diagnostics.Category
}

// addOHSDKDiagnostic is the Go equivalent of checker.ts::collectDiagnostics:
// SDK callbacks retain TS28007 as their carrier code while replacing its text
// and category with the callback result.
func (c *Checker) addOHSDKDiagnostic(node *ast.Node, result ohSDKCheckResult) {
	file := ast.GetSourceFileOfNode(node)
	diagnostic := ast.NewDiagnosticFromText(
		file,
		scanner.GetErrorRangeForNode(file, node),
		diagnostics.This_API_has_been_Special_Markings_exercise_caution_when_using_this_API.Code(),
		result.category,
		result.message,
		nil,
		nil,
		false,
		false,
	)
	c.addDiagnostic(diagnostic)
	c.addSuggestionDiagnostic(diagnostic)
}

// checkApiAvailableVersion ports checker.ts::checkApiAvailableVersion and
// ets2bundle api_check_utils::isApiAvailableVersionSpecifications. It checks
// only the SDK declaration identity; a user object with the same member name
// is intentionally ignored.
func (c *Checker) checkApiAvailableVersion(node *ast.Node) {
	if !ast.IsCallExpression(node) || node.Expression() == nil {
		return
	}
	useFile := ast.GetSourceFileOfNode(node)
	if useFile.ScriptKind != core.ScriptKindETS && !c.compilerOptions.UsesOHModuleResolution() {
		return
	}
	expression := node.Expression()
	if ast.IsPropertyAccessExpression(expression) {
		name := expression.Name()
		if !ast.IsIdentifier(name) || name.Text() != ohApiAvailableName {
			return
		}
	}
	if len(node.Arguments()) != 1 || !c.isOHApiAvailableExpression(expression) {
		return
	}
	result := c.validateOHApiAvailableArgument(node.Arguments()[0])
	if !result.valid {
		c.addOHSDKDiagnostic(node, result)
	}
}

func (c *Checker) isOHApiAvailableExpression(node *ast.Node) bool {
	t := c.getTypeOfNode(node)
	// ets2bundle's isApiAvailableGetTypeOfNodeStatement applies
	// findNonNullType first: a nullable union is accepted only when removing
	// null/undefined leaves exactly one constituent.
	if t != nil && t.IsUnion() {
		var nonNullable *Type
		for _, constituent := range t.AsUnionType().types {
			if constituent.flags&TypeFlagsNullable != 0 {
				continue
			}
			if nonNullable != nil {
				return false
			}
			nonNullable = constituent
		}
		t = nonNullable
	}
	if t == nil || t.symbol == nil || t.symbol.ValueDeclaration == nil {
		return false
	}
	declaration := t.symbol.ValueDeclaration
	name := declaration.Name()
	return name != nil && ast.IsIdentifier(name) && name.Text() == ohApiAvailableName &&
		strings.HasSuffix(tspath.NormalizePath(ast.GetSourceFileOfNode(declaration).FileName()), ohDeviceInfoFileName)
}

func (c *Checker) validateOHApiAvailableArgument(argument *ast.Node) ohSDKCheckResult {
	result := ohSDKCheckResult{valid: true, message: ohApiAvailableError, category: diagnostics.CategoryError}
	isNumber := ast.IsNumericLiteral(argument)
	if ast.IsPrefixUnaryExpression(argument) && ast.IsNumericLiteral(argument.AsPrefixUnaryExpression().Operand) {
		operator := argument.AsPrefixUnaryExpression().Operator
		isNumber = operator == ast.KindMinusToken || operator == ast.KindPlusToken
	}
	isString := ast.IsStringLiteral(argument) || argument.Kind == ast.KindNoSubstitutionTemplateLiteral
	isNullish := argument.Kind == ast.KindNullKeyword || ast.IsIdentifier(argument) && argument.Text() == "undefined"
	// This is source behavior: non-literal expressions are accepted here and
	// remain governed by ordinary overload/type checking.
	if !isNumber && !isString && !isNullish {
		return result
	}
	if isNullish {
		result.valid = false
		result.message = ohSDKErrorMessage(ohApiAvailableError, "Null and undefined are not allowed for parameters.")
		return result
	}
	if isNumber {
		text := strings.TrimSpace(scanner.GetTextOfNode(argument))
		if !ohDecimalIntegerPattern.MatchString(text) {
			result.valid = false
			result.message = ohSDKErrorMessage(ohApiAvailableError, "Only decimal digits are allowed.")
			return result
		}
		value, parseErr := strconv.ParseFloat(text, 64)
		if !ohCanonicalDecimalIntegerPattern.MatchString(text) || parseErr != nil || value < 1 || value >= ohMSFIntegerVersion {
			result.valid = false
			contentError := ohApiAvailableDistributionContentError
			if c.ohRuntimeOS() == ohRuntimeOS {
				contentError = ohApiAvailableOpenHarmonyContentError
			}
			result.message = ohSDKErrorMessage(ohApiAvailableError, contentError)
		}
		return result
	}

	content := argument.Text()
	if c.ohRuntimeOS() == ohRuntimeOS {
		if !ohOpenHarmonyStringPattern.MatchString(content) {
			result.valid = false
			result.message = ohSDKErrorMessage(ohApiAvailableError, "Only digits and dots are allowed.")
			return result
		}
		match := ohMSFPattern.FindStringSubmatch(content)
		if len(match) == 0 || parseDecimal(match[1]) < ohMSFIntegerVersion {
			result.valid = false
			result.message = ohSDKErrorMessage(ohApiAvailableError, ohApiAvailableOpenHarmonyContentError)
		}
		return result
	}

	if !ohDistributionStringPattern.MatchString(content) {
		result.valid = false
		result.message = ohSDKErrorMessage(ohApiAvailableError, "Only digits, dots, and left and right parentheses are allowed.")
		return result
	}
	match := ohMSFPattern.FindStringSubmatch(content)
	if len(match) == 0 || parseDecimal(match[1]) >= ohMSFIntegerVersion && match[4] != "" {
		result.valid = false
		result.message = ohSDKErrorMessage(ohApiAvailableError, ohApiAvailableDistributionContentError)
		return result
	}
	if parseDecimal(match[1]) < ohMSFIntegerVersion {
		distribution, configured := c.ohSdkCheckDistribution("since", content)
		if configured && distribution.Valid {
			return result
		}
		result.valid = false
		result.message = "11706014#" + distribution.Message
	}
	return result
}

func ohSDKErrorMessage(base string, suffix string) string {
	return "11706013#" + base + suffix
}

func (c *Checker) ohRuntimeOS() string {
	if c.compilerOptions.OhRuntimeOS != "" {
		return c.compilerOptions.OhRuntimeOS
	}
	return ohRuntimeOS
}

func parseDecimal(value string) int {
	result, _ := strconv.Atoi(value)
	return result
}

func parseOHAvailableVersion(raw string) ohParsedVersion {
	trimmed := strings.TrimSpace(raw)
	parts := strings.Fields(trimmed)
	os, version := ohRuntimeOS, trimmed
	if len(parts) >= 2 {
		os, version = parts[0], parts[1]
	}
	return ohParsedVersion{os: os, version: version, formatVersion: os + " " + version}
}

func (c *Checker) validateOHAvailableVersion(version ohParsedVersion) ohSDKCheckResult {
	runtimeOS := c.ohRuntimeOS()
	result := ohSDKCheckResult{valid: true, category: diagnostics.CategoryError}
	if version.os != ohRuntimeOS && version.os != runtimeOS {
		result.valid = false
		result.message = "11706017#" + strings.NewReplacer(
			"$RUNTIMEOS", runtimeOS,
			"$OSNAME", version.os,
		).Replace(ohAvailableOSNameError)
		return result
	}
	checkedVersion := version.version
	valid := false
	pluginMessage := ""
	if version.os == ohRuntimeOS {
		valid = ohAvailableFormatPattern.MatchString(checkedVersion)
		if valid && strings.Contains(checkedVersion, ".") {
			valid = parseDecimal(strings.SplitN(checkedVersion, ".", 2)[0]) >= ohMSFIntegerVersion
		}
	} else {
		checkedVersion = version.formatVersion
		if pluginResult, configured := c.ohSdkCheckFormat("available", checkedVersion); configured {
			valid = pluginResult.Result
			pluginMessage = pluginResult.Message
		} else {
			valid = ohAvailableFormatPattern.MatchString(checkedVersion)
			if valid && strings.Contains(checkedVersion, ".") {
				valid = parseDecimal(strings.SplitN(checkedVersion, ".", 2)[0]) >= ohMSFIntegerVersion
			}
		}
	}
	if valid {
		return result
	}
	result.valid = false
	prefix := strings.NewReplacer("$RUNTIMEOS", runtimeOS, "$VERSION", version.version).Replace(ohAvailableVersionFormatErrorPrefix)
	if pluginMessage == "" {
		pluginMessage = ohAvailableVersionFormatError
	}
	result.message = "11706016#" + prefix + " " + pluginMessage
	return result
}

func (c *Checker) checkSourceRetentionAnnotationContent(node *ast.Node, declaration *ast.Node) {
	if declaration == nil || declaration.Name() == nil || declaration.Name().Text() != "Available" ||
		!strings.HasSuffix(tspath.NormalizePath(ast.GetSourceFileOfNode(declaration).FileName()), ohAvailableFileName) {
		return
	}
	expression := node.Expression()
	if !ast.IsCallExpression(expression) || len(expression.Arguments()) == 0 || !ast.IsObjectLiteralExpression(expression.Arguments()[0]) {
		return
	}
	for _, property := range expression.Arguments()[0].AsObjectLiteralExpression().Properties.Nodes {
		if !ast.IsPropertyAssignment(property) || property.Name() == nil || property.Name().Text() != "minApiVersion" || !ast.IsStringLiteral(property.Initializer()) {
			continue
		}
		current := parseOHAvailableVersion(property.Initializer().Text())
		if result := c.validateOHAvailableVersion(current); !result.valid {
			c.addOHSDKDiagnostic(node, result)
			return
		}
		if outer, ok := c.findOuterOHAvailableVersion(node.Parent); ok && !c.ohAvailableVersionsCompatible(outer, current) {
			c.addOHSDKDiagnostic(node, ohSDKCheckResult{
				valid:    false,
				message:  strings.Replace(ohAvailableScopeError, "$VERSION", outer.version, 1),
				category: diagnostics.CategoryWarning,
			})
		}
		return
	}
}

func (c *Checker) ohAvailableVersionsCompatible(required ohParsedVersion, target ohParsedVersion) bool {
	scene := 2
	if target.os == ohRuntimeOS {
		scene = 1
	}
	if result, configured := c.ohSdkCheckValue("available", required.formatVersion, target.formatVersion, scene); configured {
		return result.Result
	}
	return compareOHSDKVersions(target.version, required.version) >= 0
}

func (c *Checker) findOuterOHAvailableVersion(node *ast.Node) (ohParsedVersion, bool) {
	for current := node.Parent; current != nil; current = current.Parent {
		for _, modifier := range current.ModifierNodes() {
			if !ast.IsDecorator(modifier) {
				continue
			}
			expression := modifier.Expression()
			if !ast.IsCallExpression(expression) || !ast.IsIdentifier(expression.Expression()) || expression.Expression().Text() != "Available" ||
				len(expression.Arguments()) == 0 || !ast.IsObjectLiteralExpression(expression.Arguments()[0]) {
				continue
			}
			for _, property := range expression.Arguments()[0].AsObjectLiteralExpression().Properties.Nodes {
				if !ast.IsPropertyAssignment(property) || property.Name() == nil || property.Name().Text() != "minApiVersion" {
					continue
				}
				initializer := property.Initializer()
				if ast.IsStringLiteral(initializer) || ast.IsNumericLiteral(initializer) {
					version := parseOHAvailableVersion(initializer.Text())
					if c.validateOHAvailableVersion(version).valid {
						return version, true
					}
				}
			}
		}
	}
	return ohParsedVersion{}, false
}

func compareOHPointVersions(first string, second string) int {
	parse := func(value string) [3]int {
		parts := strings.Split(strings.TrimSpace(value), ".")
		var result [3]int
		for i := 0; i < len(parts) && i < len(result); i++ {
			prefix := parts[i]
			for j, ch := range prefix {
				if ch < '0' || ch > '9' {
					prefix = prefix[:j]
					break
				}
			}
			result[i] = parseDecimal(prefix)
		}
		return result
	}
	a, b := parse(first), parse(second)
	for i := range a {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	return 0
}
