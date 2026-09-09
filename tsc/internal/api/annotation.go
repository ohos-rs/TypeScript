package api

import (
	"context"
	"math"
	"strconv"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/checker"
	"github.com/microsoft/TypeScript/tsc/internal/jsnum"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
)

type AnnotationInfoResponse struct {
	Declaration     NodeHandle                    `json:"declaration"`
	SourceRetention bool                          `json:"sourceRetention"`
	Properties      []*AnnotationPropertyResponse `json:"properties"`
}

type AnnotationTransformInfoResponse struct {
	FileName     string                               `json:"fileName"`
	Declarations []*AnnotationTransformNodeResponse   `json:"declarations"`
	Uses         []*AnnotationTransformNodeResponse   `json:"uses"`
	Imports      []*AnnotationTransformImportResponse `json:"imports"`
}

type AnnotationTransformNodeResponse struct {
	Pos             int                     `json:"pos"`
	End             int                     `json:"end"`
	RuntimeRetained bool                    `json:"runtimeRetained"`
	Info            *AnnotationInfoResponse `json:"info"`
}

type AnnotationTransformImportResponse struct {
	Pos         int    `json:"pos"`
	End         int    `json:"end"`
	Disposition string `json:"disposition"`
}

type AnnotationPropertyResponse struct {
	Name            string                      `json:"name"`
	NameText        string                      `json:"nameText"`
	Declaration     NodeHandle                  `json:"declaration"`
	Type            *TypeResponse               `json:"type"`
	Initializer     *AnnotationConstantResponse `json:"initializer"`
	Argument        *AnnotationConstantResponse `json:"argument"`
	ArrayDepth      int                         `json:"arrayDepth"`
	ElementType     *TypeResponse               `json:"elementType"`
	EnumDeclaration NodeHandle                  `json:"enumDeclaration,omitempty"`
	EnumFirstValue  *AnnotationConstantResponse `json:"enumFirstValue"`
	TypeText        string                      `json:"typeText"`
	ElementTypeText string                      `json:"elementTypeText"`
}

// Scalar text is tagged so JSON cannot erase -0 or reject NaN/Infinity.
// Nil represents no evaluated constant; an empty array is kind=array/items=[].
type AnnotationConstantResponse struct {
	Kind  string                        `json:"kind"`
	Value string                        `json:"value"`
	Items []*AnnotationConstantResponse `json:"items,omitzero"`
}

func (r *AnnotationConstantResponse) set(value any) *AnnotationConstantResponse {
	switch value := value.(type) {
	case jsnum.Number:
		r.Kind = "number"
		r.Value = strconv.FormatFloat(float64(value), 'g', -1, 64)
		if math.IsInf(float64(value), 1) {
			r.Value = "Infinity"
		}
		if math.IsInf(float64(value), -1) {
			r.Value = "-Infinity"
		}
	case string:
		r.Kind, r.Value = "string", value
	case bool:
		r.Kind, r.Value = "boolean", strconv.FormatBool(value)
	case []any:
		r.Kind = "array"
		r.Items = make([]*AnnotationConstantResponse, len(value))
		for i, element := range value {
			r.Items[i] = new(AnnotationConstantResponse).set(element)
		}
	default:
		return nil
	}
	return r
}

// @gen-proto-nullable
func (s *Session) handleGetAnnotationInfo(ctx context.Context, params *CheckerNodeParams) (*AnnotationInfoResponse, error) {
	setup, err := s.setupChecker(ctx, params.Snapshot, params.Project)
	if err != nil {
		return nil, err
	}
	defer setup.done()
	node, err := setup.sd.resolveNodeHandle(setup.program, params.Location)
	if err != nil {
		return nil, err
	}
	info := setup.checker.GetAnnotationInfo(node)
	if info == nil {
		return nil, nil
	}
	return setup.annotationInfoResponse(info), nil
}

func (setup checkerSetup) annotationInfoResponse(info *checker.AnnotationInfo) *AnnotationInfoResponse {
	response := &AnnotationInfoResponse{
		Declaration:     setup.sd.nodeHandleFrom(info.Declaration),
		SourceRetention: info.SourceRetention,
		Properties:      make([]*AnnotationPropertyResponse, len(info.Properties)),
	}
	for i, property := range info.Properties {
		name, _ := ast.TryGetTextOfPropertyName(property.Declaration.Name())
		response.Properties[i] = &AnnotationPropertyResponse{
			Name:            name,
			NameText:        scanner.GetTextOfNode(property.Declaration.Name()),
			Declaration:     setup.sd.nodeHandleFrom(property.Declaration),
			Type:            setup.newTypeResponse(property.Type),
			Initializer:     new(AnnotationConstantResponse).set(property.Initializer),
			Argument:        new(AnnotationConstantResponse).set(property.Argument),
			ArrayDepth:      property.ArrayDepth,
			ElementType:     setup.newTypeResponse(property.ElementType),
			EnumFirstValue:  new(AnnotationConstantResponse).set(property.EnumFirstValue),
			TypeText:        setup.checker.TypeToStringEx(property.Type, property.Declaration, checker.TypeFormatFlagsAllowUniqueESSymbolType|checker.TypeFormatFlagsUseAliasDefinedOutsideCurrentScope, nil),
			ElementTypeText: setup.checker.TypeToStringEx(property.ElementType, property.Declaration, checker.TypeFormatFlagsAllowUniqueESSymbolType|checker.TypeFormatFlagsUseAliasDefinedOutsideCurrentScope, nil),
		}
		if property.EnumDeclaration != nil {
			response.Properties[i].EnumDeclaration = setup.sd.nodeHandleFrom(property.EnumDeclaration)
		}
	}
	return response
}

// handleGetAnnotationTransformInfos batches the checker-owned facts consumed by
// OH ohApi.ts::transformAnnotation. Positions are UTF-16 protocol offsets,
// matching every other public position-bearing API response.
func (s *Session) handleGetAnnotationTransformInfos(ctx context.Context, params *SelectedFilesEmitParams) ([]*AnnotationTransformInfoResponse, error) {
	sd, program, err := s.setupProgram(params.Snapshot, params.Project)
	if err != nil {
		return nil, err
	}
	result := make([]*AnnotationTransformInfoResponse, 0, len(params.Files))
	for _, file := range params.Files {
		sourceFile := program.GetSourceFile(file.ToFileName())
		if sourceFile == nil {
			continue
		}
		fileChecker, done := program.GetTypeCheckerForFileExclusive(ctx, sourceFile)
		setup := checkerSetup{sd: sd, program: program, checker: fileChecker, done: done, projectID: params.Project}
		result = append(result, setup.annotationTransformInfoResponse(sourceFile))
		done()
	}
	return result, nil
}

func (setup checkerSetup) annotationTransformInfoResponse(sourceFile *ast.SourceFile) *AnnotationTransformInfoResponse {
	positions := sourceFile.GetPositionMap()
	response := &AnnotationTransformInfoResponse{
		FileName:     sourceFile.FileName(),
		Declarations: make([]*AnnotationTransformNodeResponse, 0),
		Uses:         make([]*AnnotationTransformNodeResponse, 0),
		Imports:      make([]*AnnotationTransformImportResponse, 0),
	}
	var visit func(*ast.Node) bool
	visit = func(node *ast.Node) bool {
		switch {
		case ast.IsAnnotationDeclaration(node):
			if info := setup.checker.GetAnnotationInfo(node); info != nil {
				response.Declarations = append(response.Declarations, &AnnotationTransformNodeResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					Info: setup.annotationInfoResponse(info),
				})
			}
		case ast.IsDecorator(node):
			if info := setup.checker.GetAnnotationInfo(node); info != nil {
				owner := node.Parent
				runtimeRetained := owner != nil && ((ast.IsClassDeclaration(owner) &&
					!ast.IsAnnotationDeclaration(owner) &&
					!ast.IsStructDeclaration(owner)) || ast.IsMethodDeclaration(owner))
				response.Uses = append(response.Uses, &AnnotationTransformNodeResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					RuntimeRetained: runtimeRetained,
					Info:            setup.annotationInfoResponse(info),
				})
			}
		case ast.IsImportSpecifier(node):
			disposition := setup.checker.GetAnnotationImportDisposition(node)
			if disposition != checker.AnnotationImportUnchanged {
				response.Imports = append(response.Imports, &AnnotationTransformImportResponse{
					Pos: positions.UTF8ToUTF16(node.Pos()), End: positions.UTF8ToUTF16(node.End()),
					Disposition: string(disposition),
				})
			}
		}
		node.ForEachChild(visit)
		return false
	}
	sourceFile.AsNode().ForEachChild(visit)
	return response
}
