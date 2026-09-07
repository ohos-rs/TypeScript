package api

import (
	"context"
	"math"
	"strconv"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/jsnum"
)

type AnnotationInfoResponse struct {
	Declaration     NodeHandle                    `json:"declaration"`
	SourceRetention bool                          `json:"sourceRetention"`
	Properties      []*AnnotationPropertyResponse `json:"properties"`
}

type AnnotationPropertyResponse struct {
	Name            string                      `json:"name"`
	Declaration     NodeHandle                  `json:"declaration"`
	Type            *TypeResponse               `json:"type"`
	Initializer     *AnnotationConstantResponse `json:"initializer"`
	Argument        *AnnotationConstantResponse `json:"argument"`
	ArrayDepth      int                         `json:"arrayDepth"`
	ElementType     *TypeResponse               `json:"elementType"`
	EnumDeclaration NodeHandle                  `json:"enumDeclaration,omitempty"`
	EnumFirstValue  *AnnotationConstantResponse `json:"enumFirstValue"`
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
	response := &AnnotationInfoResponse{
		Declaration:     setup.sd.nodeHandleFrom(info.Declaration),
		SourceRetention: info.SourceRetention,
		Properties:      make([]*AnnotationPropertyResponse, len(info.Properties)),
	}
	for i, property := range info.Properties {
		name, _ := ast.TryGetTextOfPropertyName(property.Declaration.Name())
		response.Properties[i] = &AnnotationPropertyResponse{
			Name:           name,
			Declaration:    setup.sd.nodeHandleFrom(property.Declaration),
			Type:           setup.newTypeResponse(property.Type),
			Initializer:    new(AnnotationConstantResponse).set(property.Initializer),
			Argument:       new(AnnotationConstantResponse).set(property.Argument),
			ArrayDepth:     property.ArrayDepth,
			ElementType:    setup.newTypeResponse(property.ElementType),
			EnumFirstValue: new(AnnotationConstantResponse).set(property.EnumFirstValue),
		}
		if property.EnumDeclaration != nil {
			response.Properties[i].EnumDeclaration = setup.sd.nodeHandleFrom(property.EnumDeclaration)
		}
	}
	return response, nil
}
