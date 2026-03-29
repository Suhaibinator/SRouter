package router

import (
	"reflect"
	"strings"
)

const maxSchemaDepth = 10

func buildTypeSchema(t reflect.Type) *TypeSchema {
	return buildTypeSchemaDepth(t, 0)
}

func buildTypeSchemaDepth(t reflect.Type, depth int) *TypeSchema {
	if t == nil || depth > maxSchemaDepth {
		return nil
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		if t == nil {
			return nil
		}
	}

	schema := &TypeSchema{
		TypeName: t.Name(),
		Package:  t.PkgPath(),
		Kind:     strings.ToLower(t.Kind().String()),
	}

	if t.Kind() != reflect.Struct {
		return schema
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldSchema := FieldSchema{
			Name: field.Name,
			Type: field.Type.String(),
		}

		tag := field.Tag.Get("json")
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" && parts[0] != "-" {
				fieldSchema.JSONName = parts[0]
			}
			fieldSchema.Required = !contains(parts[1:], "omitempty")
		}

		nestedType := field.Type
		for nestedType.Kind() == reflect.Pointer {
			nestedType = nestedType.Elem()
		}
		switch nestedType.Kind() {
		case reflect.Struct:
			fieldSchema.Schema = buildTypeSchemaDepth(nestedType, depth+1)
		case reflect.Slice, reflect.Array:
			elem := nestedType.Elem()
			for elem.Kind() == reflect.Pointer {
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				fieldSchema.Schema = buildTypeSchemaDepth(elem, depth+1)
			}
		case reflect.Map:
			elem := nestedType.Elem()
			for elem.Kind() == reflect.Pointer {
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				fieldSchema.Schema = buildTypeSchemaDepth(elem, depth+1)
			}
		}

		schema.Fields = append(schema.Fields, fieldSchema)
	}

	return schema
}

func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
