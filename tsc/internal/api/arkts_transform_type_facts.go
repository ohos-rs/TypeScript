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
	IsNullable bool                            `json:"isNullable"`
	IsEnum     bool                            `json:"isEnum"`
	IsBasic    bool                            `json:"isBasic"`
	SymbolName string                          `json:"symbolName"`
	Types      []*ResolvedTypeIdentityResponse `json:"types"`
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
		IsEnum:     flags&checker.TypeFlagsEnumLike != 0,
		IsBasic:    flags&basic != 0,
	}
	if symbol := typ.Symbol(); symbol != nil {
		response.SymbolName = symbol.Name
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
func (s *Session) handleGetArkTSTransformTypeFacts(ctx context.Context, params *SelectedFilesEmitParams) ([]*ArkTSTransformTypeFactsResponse, error) {
	setup, err := s.setupChecker(ctx, params.Snapshot, params.Project)
	if err != nil {
		return nil, err
	}
	defer setup.done()
	result := make([]*ArkTSTransformTypeFactsResponse, 0, len(params.Files))
	for _, file := range params.Files {
		sourceFile := setup.program.GetSourceFile(file.ToFileName())
		if sourceFile == nil {
			continue
		}
		result = append(result, setup.arkTSTransformTypeFactsResponse(sourceFile))
	}
	return result, nil
}

func (setup checkerSetup) arkTSTransformTypeFactsResponse(sourceFile *ast.SourceFile) *ArkTSTransformTypeFactsResponse {
	positions := sourceFile.GetPositionMap()
	response := &ArkTSTransformTypeFactsResponse{
		FileName:        sourceFile.FileName(),
		Properties:      make([]*ArkTSPropertyTypeFactsResponse, 0),
		BuilderAccesses: make([]*ArkTSBuilderReceiverTypeFactsResponse, 0),
		MemberAccesses:  make([]*ArkTSExpressionTypeFactsResponse, 0),
	}
	var visit func(*ast.Node) bool
	visit = func(node *ast.Node) bool {
		if ast.IsPropertyDeclaration(node) && (node.Parent == nil || !ast.IsAnnotationDeclaration(node.Parent)) {
			property := &ArkTSPropertyTypeFactsResponse{
				NamePos: positions.UTF8ToUTF16(node.Name().Pos()),
				NameEnd: positions.UTF8ToUTF16(node.Name().End()),
			}
			if typeNode := node.Type(); typeNode != nil {
				property.Type = newResolvedTypeIdentityResponse(setup.checker.GetTypeFromTypeNode(typeNode))
			}
			response.Properties = append(response.Properties, property)
		}
		if ast.IsPropertyAccessExpression(node) {
			if node.Name().Text() == "builder" {
				response.BuilderAccesses = append(response.BuilderAccesses, &ArkTSBuilderReceiverTypeFactsResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					ReceiverType: newResolvedTypeIdentityResponse(setup.checker.GetTypeAtLocation(node.Expression())),
				})
			}
			// ets2bundle's type-checker fallback in isRegularAttrNode is only
			// reached for `Identifier.Member`, so do not grow the snapshot with
			// unrelated chained and `this` accesses.
			if ast.IsIdentifier(node.Expression()) {
				response.MemberAccesses = append(response.MemberAccesses, &ArkTSExpressionTypeFactsResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					Type: newResolvedTypeIdentityResponse(setup.checker.GetTypeAtLocation(node)),
				})
			}
		}
		node.ForEachChild(visit)
		return false
	}
	sourceFile.AsNode().ForEachChild(visit)
	return response
}
