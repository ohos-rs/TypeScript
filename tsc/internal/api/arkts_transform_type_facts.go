package api

import (
	"context"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/checker"
)

// ArkTSTransformTypeFactsResponse contains checker results only. The native
// consumer owns all emitted syntax and applies ets2bundle's transform rules.
type ArkTSTransformTypeFactsResponse struct {
	FileName        string                                   `json:"fileName"`
	Properties      []*ArkTSPropertyTypeFactsResponse        `json:"properties"`
	BuilderAccesses []*ArkTSBuilderReceiverTypeFactsResponse `json:"builderAccesses"`
	MemberAccesses  []*ArkTSExpressionTypeFactsResponse      `json:"memberAccesses"`
	SDKApiUses      []*ArkTSSDKApiUseResponse                `json:"sdkApiUses"`
}

// ArkTSTransformTypeFactsParams contains syntax-selected type queries. The
// native OXC transform owns node discovery; this service only resolves types.
type ArkTSTransformTypeFactsParams struct {
	Snapshot       SnapshotID                          `json:"snapshot"`
	Project        ProjectID                           `json:"project"`
	Queries        []*ArkTSTransformTypeFactsFileQuery `json:"queries"`
	SDKApiUseFiles []DocumentIdentifier                `json:"sdkApiUseFiles"`
}

type ArkTSTransformTypeFactsFileQuery struct {
	File            DocumentIdentifier     `json:"file"`
	Properties      []*ArkTSTypeQueryRange `json:"properties"`
	BuilderAccesses []*ArkTSTypeQueryRange `json:"builderAccesses"`
	MemberAccesses  []*ArkTSTypeQueryRange `json:"memberAccesses"`
}

// ArkTSTypeQueryRange positions are UTF-16 offsets, matching the public API
// protocol and the spans returned to the Rust consumer.
type ArkTSTypeQueryRange struct {
	Pos int `json:"pos"`
	End int `json:"end"`
}

type ArkTSSDKApiUseResponse struct {
	ApiModule string `json:"apiModule"`
	Function  string `json:"function"`
}

type ArkTSPropertyTypeFactsResponse struct {
	NamePos int                           `json:"namePos"`
	NameEnd int                           `json:"nameEnd"`
	Type    *ResolvedTypeIdentityResponse `json:"type,omitempty"`
}

type ArkTSBuilderReceiverTypeFactsResponse struct {
	Pos          int                           `json:"pos"`
	End          int                           `json:"end"`
	ReceiverType *ResolvedTypeIdentityResponse `json:"receiverType"`
}

type ArkTSExpressionTypeFactsResponse struct {
	Pos  int                           `json:"pos"`
	End  int                           `json:"end"`
	Type *ResolvedTypeIdentityResponse `json:"type"`
}

// ResolvedTypeIdentityResponse is deliberately a value snapshot rather than
// an API object handle. One batched request therefore has no follow-up IPC and
// remains valid after the Program snapshot is released.
type ResolvedTypeIdentityResponse struct {
	IsNullable   bool                            `json:"isNullable"`
	IsUnion      bool                            `json:"isUnion"`
	IsEnum       bool                            `json:"isEnum"`
	IsBasic      bool                            `json:"isBasic"`
	IsObservedV2 bool                            `json:"isObservedV2"`
	IsFunction   bool                            `json:"isFunction"`
	SymbolName   string                          `json:"symbolName"`
	Types        []*ResolvedTypeIdentityResponse `json:"types"`
}

func newResolvedTypeIdentityResponse(typ *checker.Type) *ResolvedTypeIdentityResponse {
	if typ == nil {
		return nil
	}
	flags := typ.Flags()
	const basic = checker.TypeFlagsString | checker.TypeFlagsNumber | checker.TypeFlagsBoolean |
		checker.TypeFlagsEnum | checker.TypeFlagsBigInt | checker.TypeFlagsStringLiteral |
		checker.TypeFlagsNumberLiteral | checker.TypeFlagsBooleanLiteral |
		checker.TypeFlagsEnumLiteral | checker.TypeFlagsBigIntLiteral
	response := &ResolvedTypeIdentityResponse{
		IsNullable: flags&checker.TypeFlagsNullable != 0,
		IsUnion:    flags&checker.TypeFlagsUnion != 0,
		IsEnum:     flags&checker.TypeFlagsEnumLike != 0,
		IsBasic:    flags&basic != 0,
	}
	if symbol := typ.Symbol(); symbol != nil {
		response.SymbolName = symbol.Name
		response.IsFunction = symbol.Name == "Function"
		for _, declaration := range symbol.Declarations {
			if ast.IsClassDeclaration(declaration) && ast.HasArkUIBareDecorator(declaration.Modifiers(), "ObservedV2") {
				response.IsObservedV2 = true
				break
			}
		}
	}
	if flags&checker.TypeFlagsUnionOrIntersection != 0 {
		for _, constituent := range typ.Types() {
			response.Types = append(response.Types, newResolvedTypeIdentityResponse(constituent))
		}
	}
	return response
}

// handleGetArkTSTransformTypeFacts batches the type queries used by
// process_component_member.ts::isSimpleType and
// process_component_build.ts::{isWrappedBuilder,isMutableBuilder,isRegularAttrNode}.
func (s *Session) handleGetArkTSTransformTypeFacts(ctx context.Context, params *ArkTSTransformTypeFactsParams) ([]*ArkTSTransformTypeFactsResponse, error) {
	_, program, err := s.setupProgram(params.Snapshot, params.Project)
	if err != nil {
		return nil, err
	}
	queries := make([]*ArkTSTransformTypeFactsFileQuery, 0, len(params.Queries))
	queryFiles := make([]*ast.SourceFile, 0, len(params.Queries))
	for _, query := range params.Queries {
		sourceFile := program.GetSourceFile(query.File.ToFileName())
		if sourceFile == nil {
			continue
		}
		queries = append(queries, query)
		queryFiles = append(queryFiles, sourceFile)
	}
	result := make([]*ArkTSTransformTypeFactsResponse, len(queryFiles))
	program.ForEachArkTSBuildCheckerGroup(ctx, queryFiles, func(fileChecker *checker.Checker, index int, sourceFile *ast.SourceFile) {
		result[index] = arkTSTransformTypeFactsResponse(fileChecker, sourceFile, queries[index])
	})
	responseIndexes := make(map[string]int, len(result))
	for index, response := range result {
		responseIndexes[response.FileName] = index
	}

	// SDK API use facts belong to the checker instance. They cannot be reused
	// across checker instances, so cross-platform builds collect them from the
	// same compiler checker partition that owns diagnostics and transform type
	// queries. Normal builds pass no SDKApiUseFiles and perform no extra work.
	sdkFiles := make([]*ast.SourceFile, 0, len(params.SDKApiUseFiles))
	for _, file := range params.SDKApiUseFiles {
		sourceFile := program.GetSourceFile(file.ToFileName())
		if sourceFile == nil {
			continue
		}
		sdkFiles = append(sdkFiles, sourceFile)
	}
	sdkUses := make([][]checker.OHSDKUseFact, len(sdkFiles))
	program.ForEachArkTSBuildCheckerGroup(ctx, sdkFiles, func(fileChecker *checker.Checker, index int, sourceFile *ast.SourceFile) {
		fileChecker.GetDiagnostics(ctx, sourceFile)
		sdkUses[index] = fileChecker.OHSDKUseFacts(sourceFile.FileName())
	})
	for index, uses := range sdkUses {
		if len(uses) == 0 {
			continue
		}
		fileName := sdkFiles[index].FileName()
		responseIndex, ok := responseIndexes[fileName]
		if !ok {
			responseIndex = len(result)
			responseIndexes[fileName] = responseIndex
			result = append(result, newArkTSTransformTypeFactsResponse(fileName))
		}
		response := result[responseIndex]
		for _, fact := range uses {
			response.SDKApiUses = append(response.SDKApiUses, &ArkTSSDKApiUseResponse{
				ApiModule: fact.ApiModule,
				Function:  fact.Function,
			})
		}
	}
	return result, nil
}

func newArkTSTransformTypeFactsResponse(fileName string) *ArkTSTransformTypeFactsResponse {
	return &ArkTSTransformTypeFactsResponse{
		FileName:        fileName,
		Properties:      make([]*ArkTSPropertyTypeFactsResponse, 0),
		BuilderAccesses: make([]*ArkTSBuilderReceiverTypeFactsResponse, 0),
		MemberAccesses:  make([]*ArkTSExpressionTypeFactsResponse, 0),
		SDKApiUses:      make([]*ArkTSSDKApiUseResponse, 0),
	}
}

func arkTSTransformTypeFactsResponse(typeChecker *checker.Checker, sourceFile *ast.SourceFile, query *ArkTSTransformTypeFactsFileQuery) *ArkTSTransformTypeFactsResponse {
	positions := sourceFile.GetPositionMap()
	response := newArkTSTransformTypeFactsResponse(sourceFile.FileName())
	propertyEnds := make(map[int]struct{}, len(query.Properties))
	for _, item := range query.Properties {
		propertyEnds[positions.UTF16ToUTF8(item.End)] = struct{}{}
	}
	builderAccessEnds := make(map[int]struct{}, len(query.BuilderAccesses))
	for _, item := range query.BuilderAccesses {
		builderAccessEnds[positions.UTF16ToUTF8(item.End)] = struct{}{}
	}
	memberAccessEnds := make(map[int]struct{}, len(query.MemberAccesses))
	for _, item := range query.MemberAccesses {
		memberAccessEnds[positions.UTF16ToUTF8(item.End)] = struct{}{}
	}
	var visit func(*ast.Node) bool
	visit = func(node *ast.Node) bool {
		if ast.IsPropertyDeclaration(node) && (node.Parent == nil || !ast.IsAnnotationDeclaration(node.Parent)) {
			if _, selected := propertyEnds[node.Name().End()]; !selected {
				node.ForEachChild(visit)
				return false
			}
			property := &ArkTSPropertyTypeFactsResponse{
				NamePos: positions.UTF8ToUTF16(node.Name().Pos()),
				NameEnd: positions.UTF8ToUTF16(node.Name().End()),
			}
			if typeNode := node.Type(); typeNode != nil {
				property.Type = newResolvedTypeIdentityResponse(typeChecker.GetTypeFromTypeNode(typeNode))
			}
			response.Properties = append(response.Properties, property)
		}
		if ast.IsPropertyAccessExpression(node) {
			if _, selected := builderAccessEnds[node.End()]; selected && node.Name().Text() == "builder" {
				response.BuilderAccesses = append(response.BuilderAccesses, &ArkTSBuilderReceiverTypeFactsResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					ReceiverType: newResolvedTypeIdentityResponse(typeChecker.GetTypeAtLocation(node.Expression())),
				})
			}
			// ets2bundle's type-checker fallback in isRegularAttrNode is only
			// reached for `Identifier.Member`, so do not grow the snapshot with
			// unrelated chained and `this` accesses.
			if _, selected := memberAccessEnds[node.End()]; selected && ast.IsIdentifier(node.Expression()) {
				response.MemberAccesses = append(response.MemberAccesses, &ArkTSExpressionTypeFactsResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					Type: newResolvedTypeIdentityResponse(typeChecker.GetTypeAtLocation(node)),
				})
			}
		}
		node.ForEachChild(visit)
		return false
	}
	if len(propertyEnds)+len(builderAccessEnds)+len(memberAccessEnds) != 0 {
		sourceFile.AsNode().ForEachChild(visit)
	}
	return response
}
