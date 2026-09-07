package core

import (
	"encoding/json"
	"iter"
	"unique"
)

// Immutable, comparable configuration sequences. Zero is absent/null, while
// [] remains present. Weak interning releases unused values and lets parser
// cache keys compare content without reparsing JSON per source file.
type EtsList[T comparable] struct {
	head unique.Handle[etsListEntry[T]]
}
type etsListEntry[T comparable] struct {
	Value T
	Next  EtsList[T]
	End   bool
}

func (l EtsList[T]) IsZero() bool { return l.head == (unique.Handle[etsListEntry[T]]{}) }
func (l EtsList[T]) Values() iter.Seq[T] {
	return func(yield func(T) bool) {
		for current := l; !current.IsZero(); {
			entry := current.head.Value()
			if entry.End || !yield(entry.Value) {
				return
			}
			current = entry.Next
		}
	}
}
func (l EtsList[T]) Contains(value T) bool {
	for item := range l.Values() {
		if item == value {
			return true
		}
	}
	return false
}
func (l EtsList[T]) MarshalJSON() ([]byte, error) {
	if l.IsZero() {
		return []byte("null"), nil
	}
	values := make([]T, 0)
	for value := range l.Values() {
		values = append(values, value)
	}
	return json.Marshal(values)
}
func (l *EtsList[T]) UnmarshalJSON(data []byte) error {
	var values []T
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*l = EtsList[T]{}
	if values == nil {
		return nil
	}
	l.head = unique.Make(etsListEntry[T]{End: true})
	for i := len(values) - 1; i >= 0; i-- {
		l.head = unique.Make(etsListEntry[T]{Value: values[i], Next: *l})
	}
	return nil
}

// Optional scalars/objects retain presence without pointer identity leaking
// into SourceFileParseOptions equality.
type EtsValue[T comparable] struct{ value unique.Handle[T] }

func (v EtsValue[T]) IsZero() bool { return v.value == (unique.Handle[T]{}) }
func (v EtsValue[T]) Get() (T, bool) {
	if v.IsZero() {
		var zero T
		return zero, false
	}
	return v.value.Value(), true
}
func (v EtsValue[T]) Or(fallback T) T {
	if value, ok := v.Get(); ok {
		return value
	}
	return fallback
}
func (v EtsValue[T]) MarshalJSON() ([]byte, error) {
	if v.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(v.value.Value())
}
func (v *EtsValue[T]) UnmarshalJSON(data []byte) error {
	var value *T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*v = EtsValue[T]{}
	if value != nil {
		v.value = unique.Make(*value)
	}
	return nil
}

// OH types.ts::EtsOptions. Preserve table order: Extend chooses the last
// decorator and attribute callbacks use the first matching component record.
type EtsOptions struct {
	Render             EtsRenderOptions              `json:"render,omitzero"`
	Components         EtsList[string]               `json:"components,omitzero"`
	Libs               EtsList[string]               `json:"libs,omitzero"`
	Extend             EtsExtendOptions              `json:"extend,omitzero"`
	Styles             EtsStylesOptions              `json:"styles,omitzero"`
	Concurrent         EtsConcurrentOptions          `json:"concurrent,omitzero"`
	CustomComponent    EtsValue[string]              `json:"customComponent,omitzero"`
	PropertyDecorators EtsList[EtsPropertyDecorator] `json:"propertyDecorators,omitzero"`
	EmitDecorators     EtsList[EtsEmitDecorator]     `json:"emitDecorators,omitzero"`
	SyntaxComponents   EtsSyntaxComponents           `json:"syntaxComponents,omitzero"`
}

func (o EtsOptions) Equal(other EtsOptions) bool { return o == other }

type EtsRenderOptions struct {
	Method    EtsList[string] `json:"method,omitzero"`
	Decorator EtsList[string] `json:"decorator,omitzero"`
}
type EtsComponentDeclaration struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Instance string `json:"instance"`
}
type EtsExtendOptions struct {
	Decorator  EtsList[string]                  `json:"decorator,omitzero"`
	Components EtsList[EtsComponentDeclaration] `json:"components,omitzero"`
}
type EtsStylesOptions struct {
	Decorator EtsValue[string]                  `json:"decorator,omitzero"`
	Component EtsValue[EtsComponentDeclaration] `json:"component,omitzero"`
	Property  EtsValue[string]                  `json:"property,omitzero"`
}
type EtsConcurrentOptions struct {
	Decorator EtsValue[string] `json:"decorator,omitzero"`
}
type EtsPropertyDecorator struct {
	Name               string `json:"name"`
	NeedInitialization bool   `json:"needInitialization"`
}
type EtsEmitDecorator struct {
	Name           string `json:"name"`
	EmitParameters bool   `json:"emitParameters"`
}
type EtsSyntaxComponents struct {
	ParamsUICallback EtsList[string]               `json:"paramsUICallback,omitzero"`
	AttrUICallback   EtsList[EtsAttributeCallback] `json:"attrUICallback,omitzero"`
}
type EtsAttributeCallback struct {
	Name       string          `json:"name"`
	Attributes EtsList[string] `json:"attributes,omitzero"`
}
