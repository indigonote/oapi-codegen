package codegen

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"
)

// This describes a Schema, a type definition.
type Schema struct {
	GoType     string // The Go type needed to represent the schema
	RefType    string // If the type has a type name, this is set
	EsTemplate string // This field use for create es index template

	EnumValues []string // Enum values

	Properties               []Property       // For an object, the fields with names
	HasAdditionalProperties  bool             // Whether we support additional properties
	AdditionalPropertiesType *Schema          // And if we do, their type
	AdditionalTypes          []TypeDefinition // We may need to generate auxiliary helper types, stored here

	SkipOptionalPointer bool // Some types don't need a * in front when they're optional
}

func (s Schema) IsRef() bool {
	return s.RefType != ""
}

func (s Schema) TypeDecl() string {
	if s.IsRef() {
		return s.RefType
	}
	return s.GoType
}

func (s Schema) EsTemplateDecl() string {
	return s.EsTemplate
}

func (s *Schema) MergeProperty(p Property) error {
	// Scan all existing properties for a conflict
	for _, e := range s.Properties {
		if e.JsonFieldName == p.JsonFieldName && !PropertiesEqual(e, p) {
			return errors.New(fmt.Sprintf("property '%s' already exists with a different type", e.JsonFieldName))
		}
	}
	s.Properties = append(s.Properties, p)
	return nil
}

func (s Schema) GetAdditionalTypeDefs() []TypeDefinition {
	var result []TypeDefinition
	for _, p := range s.Properties {
		result = append(result, p.Schema.GetAdditionalTypeDefs()...)
	}
	result = append(result, s.AdditionalTypes...)
	return result
}

type Property struct {
	Description   string
	JsonFieldName string
	Schema        Schema
	Required      bool
	Nullable      bool
	Validation    map[string]string
	EsTag         string
}

func (p Property) GoFieldName() string {
	return SchemaNameToTypeName(p.JsonFieldName)
}

func (p Property) GoTypeDef() string {
	typeDef := p.Schema.TypeDecl()
	if !p.Schema.SkipOptionalPointer && (!p.Required || p.Nullable) {
		typeDef = "*" + typeDef
	}
	return typeDef
}

type TypeDefinition struct {
	TypeName     string
	JsonName     string
	ResponseName string
	Schema       Schema
}

func PropertiesEqual(a, b Property) bool {
	return a.JsonFieldName == b.JsonFieldName && a.Schema.TypeDecl() == b.Schema.TypeDecl() && a.Required == b.Required
}

func GenerateGoSchema(sref *openapi3.SchemaRef, path []string) (Schema, error) {
	// If Ref is set on the SchemaRef, it means that this type is actually a reference to
	// another type. We're not de-referencing, so simply use the referenced type.
	var refType string

	// Add a fallback value in case the sref is nil.
	// i.e. the parent schema defines a type:array, but the array has
	// no items defined. Therefore we have at least valid Go-Code.
	if sref == nil {
		return Schema{GoType: "interface{}", RefType: refType}, nil
	}

	schema := sref.Value

	if sref.Ref != "" {
		var err error
		// Convert the reference path to Go type
		refType, err = RefPathToGoType(sref.Ref)
		if err != nil {
			return Schema{}, fmt.Errorf("error turning reference (%s) into a Go type: %s",
				sref.Ref, err)
		}
		return Schema{
			GoType: refType,
		}, nil
	}

	// We can't support this in any meaningful way
	if schema.AnyOf != nil {
		return Schema{GoType: "interface{}", RefType: refType}, nil
	}
	// We can't support this in any meaningful way
	if schema.OneOf != nil {
		return Schema{GoType: "interface{}", RefType: refType}, nil
	}

	// AllOf is interesting, and useful. It's the union of a number of other
	// schemas. A common usage is to create a union of an object with an ID,
	// so that in a RESTful paradigm, the Create operation can return
	// (object, id), so that other operations can refer to (id)
	if schema.AllOf != nil {
		mergedSchema, err := MergeSchemas(schema.AllOf, path)
		if err != nil {
			return Schema{}, errors.Wrap(err, "error merging schemas")
		}
		mergedSchema.RefType = refType
		return mergedSchema, nil
	}

	// Schema type and format, eg. string / binary
	t := schema.Type

	outSchema := Schema{
		RefType: refType,
	}
	// Handle objects and empty schemas first as a special case
	if t == "" || t == "object" {
		var outType string

		if len(schema.Properties) == 0 && !SchemaHasAdditionalProperties(schema) {
			// If the object has no properties or additional properties, we
			// have some special cases for its type.
			if t == "object" {
				// We have an object with no properties. This is a generic object
				// expressed as a map.
				outType = "map[string]interface{}"
			} else { // t == ""
				// If we don't even have the object designator, we're a completely
				// generic type.
				outType = "interface{}"
			}
			outSchema.GoType = outType
		} else {
			// We've got an object with some properties.
			for _, pName := range SortedSchemaKeys(schema.Properties) {
				p := schema.Properties[pName]
				propertyPath := append(path, pName)
				pSchema, err := GenerateGoSchema(p, propertyPath)
				if err != nil {
					return Schema{}, errors.Wrap(err, fmt.Sprintf("error generating Go schema for property '%s'", pName))
				}

				required := StringInArray(pName, schema.Required)

				if pSchema.HasAdditionalProperties && pSchema.RefType == "" {
					// If we have fields present which have additional properties,
					// but are not a pre-defined type, we need to define a type
					// for them, which will be based on the field names we followed
					// to get to the type.
					typeName := PathToTypeName(propertyPath)

					typeDef := TypeDefinition{
						TypeName: typeName,
						JsonName: strings.Join(propertyPath, "."),
						Schema:   pSchema,
					}
					pSchema.AdditionalTypes = append(pSchema.AdditionalTypes, typeDef)

					pSchema.RefType = typeName
				}
				description := ""
				if p.Value != nil {
					description = p.Value.Description
				}
				v := parseValidateRule(p.Value, required)
				prop := Property{
					JsonFieldName: pName,
					Schema:        pSchema,
					Required:      required,
					Description:   description,
					Nullable:      p.Value.Nullable,
					Validation:    v,
				}
				outSchema.Properties = append(outSchema.Properties, prop)
			}

			outSchema.HasAdditionalProperties = SchemaHasAdditionalProperties(schema)
			outSchema.AdditionalPropertiesType = &Schema{
				GoType: "interface{}",
			}
			if schema.AdditionalProperties != nil {
				additionalSchema, err := GenerateGoSchema(schema.AdditionalProperties, path)
				if err != nil {
					return Schema{}, errors.Wrap(err, "error generating type for additional properties")
				}
				// TODO: implement es tag here
				outSchema.AdditionalPropertiesType = &additionalSchema
			}

			outSchema.GoType = GenStructFromSchema(outSchema)
		}
		return outSchema, nil
	} else {
		f := schema.Format

		switch t {
		case "array":
			// For arrays, we'll get the type of the Items and throw a
			// [] in front of it.
			arrayType, err := GenerateGoSchema(schema.Items, path)
			if err != nil {
				return Schema{}, errors.Wrap(err, "error generating type for array")
			}
			outSchema.GoType = "[]" + arrayType.TypeDecl()
			outSchema.Properties = arrayType.Properties
		case "integer":
			// We default to int if format doesn't ask for something else.
			if f == "int64" {
				outSchema.GoType = "int64"
			} else if f == "int32" {
				outSchema.GoType = "int32"
			} else if f == "" {
				outSchema.GoType = "int"
			} else {
				return Schema{}, fmt.Errorf("invalid integer format: %s", f)
			}
		case "number":
			// We default to float for "number"
			if f == "double" {
				outSchema.GoType = "float64"
			} else if f == "float" || f == "" {
				outSchema.GoType = "float32"
			} else {
				return Schema{}, fmt.Errorf("invalid number format: %s", f)
			}
		case "boolean":
			if f != "" {
				return Schema{}, fmt.Errorf("invalid format (%s) for boolean", f)
			}
			outSchema.GoType = "bool"
		case "string":
			for _, enumValue := range schema.Enum {
				outSchema.EnumValues = append(outSchema.EnumValues, enumValue.(string))
			}
			// Special case string formats here.
			switch f {
			case "byte":
				outSchema.GoType = "[]byte"
			case "date":
				outSchema.GoType = "openapi_types.Date"
			case "date-time":
				outSchema.GoType = "time.Time"
			case "json":
				outSchema.GoType = "json.RawMessage"
				outSchema.SkipOptionalPointer = true
			default:
				// All unrecognized formats are simply a regular string.
				outSchema.GoType = "string"
			}
		default:
			return Schema{}, fmt.Errorf("unhandled Schema type: %s", t)
		}
	}
	return outSchema, nil
}

// GenerateEsSchema func do generate EsSchema
func GenerateEsSchema(sref *openapi3.SchemaRef, path []string) (Schema, error) {
	if sref == nil {
		return Schema{}, nil
	}

	schema := sref.Value

	if sref.Ref != "" {
		// need generate json template for $ref field
		// With go struct, we can only add $ref name like FhirPatient or FhirEncounter...
		// But with es json template, we cannot use this format
		template, err := GenEsTemplateFromReference(sref, path)
		if err != nil {
			return Schema{}, fmt.Errorf("error turning reference (%s) into a ElasticSearch index template: %s",
				sref.Ref, err)
		}
		return Schema{
			EsTemplate: template,
		}, nil
	}

	// We can't support this in any meaningful way
	if schema.AnyOf != nil {
		return Schema{}, nil
	}
	// We can't support this in any meaningful way
	if schema.OneOf != nil {
		return Schema{}, nil
	}

	// AllOf is interesting, and useful. It's the union of a number of other
	// schemas. A common usage is to create a union of an object with an ID,
	// so that in a RESTful paradigm, the Create operation can return
	// (object, id), so that other operations can refer to (id)
	if schema.AllOf != nil {
		tag := parseEsType(schema)
		mergedSchema, err := MergeSchemasForEs(schema.AllOf, path, tag)
		if err != nil {
			return Schema{}, errors.Wrap(err, "error merging schemas")
		}
		return mergedSchema, nil
	}

	// Schema type and format, eg. string / binary
	t := schema.Type

	outSchema := Schema{}
	// Handle objects and empty schemas first as a special case
	if t == "" || t == "object" {
		if len(schema.Properties) == 0 && !SchemaHasAdditionalProperties(schema) {
			return outSchema, nil
		}
		// We've got an object with some properties.
		for _, pName := range SortedSchemaKeys(schema.Properties) {
			p := schema.Properties[pName]
			propertyPath := append(path, pName)
			pSchema, err := GenerateEsSchema(p, propertyPath)
			if err != nil {
				return Schema{}, errors.Wrap(err, fmt.Sprintf("error generating Es schema for property '%s'", pName))
			}
			// TODO: implement later
			// if pSchema.HasAdditionalProperties && pSchema.RefType == "" {
			// 	// If we have fields present which have additional properties,
			// 	// but are not a pre-defined type, we need to define a type
			// 	// for them, which will be based on the field names we followed
			// 	// to get to the type.
			// 	typeName := PathToTypeName(propertyPath)

			// 	typeDef := TypeDefinition{
			// 		TypeName: typeName,
			// 		JsonName: strings.Join(propertyPath, "."),
			// 		Schema:   pSchema,
			// 	}
			// 	pSchema.AdditionalTypes = append(pSchema.AdditionalTypes, typeDef)

			// 	pSchema.RefType = typeName
			// }
			e := parseEsType(p.Value)
			prop := Property{
				JsonFieldName: pName,
				Schema:        pSchema,
				EsTag:         e,
			}
			outSchema.Properties = append(outSchema.Properties, prop)
		}
		outSchema.HasAdditionalProperties = SchemaHasAdditionalProperties(schema)
		outSchema.AdditionalPropertiesType = &Schema{
			GoType: "interface{}",
		}
		// TODO: implement later
		// if schema.AdditionalProperties != nil {
		// 	additionalSchema, err := GenerateEsSchema(schema.AdditionalProperties, path)
		// 	if err != nil {
		// 		return Schema{}, errors.Wrap(err, "error generating type for additional properties")
		// 	}
		//
		// 	outSchema.AdditionalPropertiesType = &additionalSchema
		// }

		if d := GenEsTemplateFromSchema(outSchema); d != "" {
			outSchema.EsTemplate = d
		}
		return outSchema, nil
	} else {
		f := schema.Format
		e := parseEsType(schema)
		if e != "" {
			parts := strings.Split(e, ",")
			templates := []string{}
			for _, v := range parts {
				if v == "fielddata" {
					templates = append(templates, `"fielddata": "true"`)
				} else if v == "text" {
					templates = append(templates, fmt.Sprintf(`"type": "%s","fields": {"keyword": {"type": "keyword", "ignore_above" : 256}}`, v))
				} else {
					templates = append(templates, fmt.Sprintf(`"type": "%s"`, v))
				}
			}
			outSchema.EsTemplate = strings.Join(templates, ",")
		}
		switch t {
		case "array":
			// For arrays, we'll get the type of the Items and throw a
			// [] in front of it.
			arrayType, err := GenerateEsSchema(schema.Items, path)
			if err != nil {
				return Schema{}, errors.Wrap(err, "error generating type for array")
			}
			// in case item is array object. type will be nested and format will like
			// "type": "nested", "properties": {"xxx": {"type": "text"}}
			if arrayType.EsTemplateDecl() != "" {
				outSchema.EsTemplate = fmt.Sprintf(`"type": "nested",%s`, arrayType.EsTemplateDecl())
			}
			outSchema.Properties = arrayType.Properties
		case "integer":
			// We default to int if format doesn't ask for something else.
			if f != "int64" && f != "int32" && f != "" {
				return Schema{}, fmt.Errorf("invalid integer format: %s", f)
			}
		case "number":
			// We default to float for "number"
			if f != "double" && f != "float" && f != "" {
				return Schema{}, fmt.Errorf("invalid number format: %s", f)
			}
		case "boolean":
			if f != "" {
				return Schema{}, fmt.Errorf("invalid format (%s) for boolean", f)
			}
		case "string":
			// Do nothing
		default:
			return Schema{}, fmt.Errorf("unhandled Schema type: %s", t)
		}
	}
	return outSchema, nil
}

// This describes a Schema, a type definition.
type SchemaDescriptor struct {
	Fields                   []FieldDescriptor
	HasAdditionalProperties  bool
	AdditionalPropertiesType string
}

type FieldDescriptor struct {
	Required bool   // Is the schema required? If not, we'll pass by pointer
	GoType   string // The Go type needed to represent the json type.
	GoName   string // The Go compatible type name for the type
	JsonName string // The json type name for the type
	IsRef    bool   // Is this schema a reference to predefined object?
}

// Given a list of schema descriptors, produce corresponding field names with
// JSON annotations
func GenFieldsFromProperties(props []Property) []string {
	var fields []string
	for _, p := range props {
		field := ""
		// Add a comment to a field in case we have one, otherwise skip.
		if p.Description != "" {
			// Separate the comment from a previous-defined, unrelated field.
			// Make sure the actual field is separated by a newline.
			field += fmt.Sprintf("\n%s\n", StringToGoComment(p.Description))
		}
		field += fmt.Sprintf("    %s %s", p.GoFieldName(), p.GoTypeDef())
		validator := ""
		if len(p.Validation) > 0 {
			s := []string{}
			if !p.Required || p.Nullable {
				s = append(s, "omitempty")
			}
			for _, v := range p.Validation {
				s = append(s, v)
			}
			validator = strings.Join(s, ",")
		}
		if p.Required || p.Nullable {
			if validator != "" {
				field += fmt.Sprintf(" `json:\"%s\" validate:\"%s\"`", p.JsonFieldName, validator)
			} else {
				field += fmt.Sprintf(" `json:\"%s\"`", p.JsonFieldName)
			}
		} else {
			if validator != "" {
				field += fmt.Sprintf(" `json:\"%s,omitempty\" validate:\"%s\"`", p.JsonFieldName, validator)
			} else {
				field += fmt.Sprintf(" `json:\"%s,omitempty\"`", p.JsonFieldName)
			}
		}
		fields = append(fields, field)
	}
	return fields
}

func GenStructFromSchema(schema Schema) string {
	// Start out with struct {
	objectParts := []string{"struct {"}
	// Append all the field definitions
	objectParts = append(objectParts, GenFieldsFromProperties(schema.Properties)...)
	// Close the struct
	if schema.HasAdditionalProperties {
		addPropsType := schema.AdditionalPropertiesType.GoType
		if schema.AdditionalPropertiesType.RefType != "" {
			addPropsType = schema.AdditionalPropertiesType.RefType
		}

		objectParts = append(objectParts,
			fmt.Sprintf("AdditionalProperties map[string]%s `json:\"-\"`", addPropsType))
	}
	objectParts = append(objectParts, "}")
	return strings.Join(objectParts, "\n")
}

// GenEsTemplateFromProperties do create properties data for es index template from OpenAPI definition properties
func GenEsTemplateFromProperties(props []Property) []string {
	var fields []string
	for _, p := range props {
		// in case estemplate is not null.
		// that mean data will be allof or array or $ref
		// with array data, format will like: {"fieldName": {"type": "nested", "properties": {"xxx": {"type": "text"}}}
		// with allof, $ref data, format will like: {"fieldName": {"properties": {"xxx": {"type": "text"}}}
		if p.Schema.EsTemplate != "" {
			field := fmt.Sprintf(`"%s": {`, p.JsonFieldName)
			field += p.Schema.EsTemplate
			field += `}`
			fields = append(fields, field)
		} else {
			// in case normal object.
			// append all properties into es index template
			if p.EsTag == "" {
				continue
			}
			field := fmt.Sprintf(`"%s":`, p.JsonFieldName)
			esTag := p.EsTag
			field += fmt.Sprintf(`{"type": "%s"}`, esTag)
			fields = append(fields, field)
		}
	}
	return fields
}

// GenEsTemplateFromSchema do generate template index from schema
// format will like `"properties": { "field1": { "type": "text" }, "field2": { "type": "text" } }`
func GenEsTemplateFromSchema(schema Schema) string {
	// Start out with "properties": {
	objectParts := []string{`"properties": {`}
	// Append all the field definitions into template format
	fields := GenEsTemplateFromProperties(schema.Properties)
	if strings.Join(fields, "") == "" {
		return ""
	}
	// TODO: handle for additional properties fields
	objectParts = append(objectParts, strings.Join(fields, ",\n"))
	objectParts = append(objectParts, "}")
	return strings.Join(objectParts, "\n")
}

// Merge all the fields in the schemas supplied into one giant schema.
func MergeSchemas(allOf []*openapi3.SchemaRef, path []string) (Schema, error) {
	var outSchema Schema
	for _, schemaOrRef := range allOf {
		ref := schemaOrRef.Ref

		var refType string
		var err error
		if ref != "" {
			refType, err = RefPathToGoType(ref)
			if err != nil {
				return Schema{}, errors.Wrap(err, "error converting reference path to a go type")
			}
		}

		schema, err := GenerateGoSchema(schemaOrRef, path)
		if err != nil {
			return Schema{}, errors.Wrap(err, "error generating Go schema in allOf")
		}
		schema.RefType = refType

		for _, p := range schema.Properties {
			err = outSchema.MergeProperty(p)
			if err != nil {
				return Schema{}, errors.Wrap(err, "error merging properties")
			}
		}

		if schema.HasAdditionalProperties {
			if outSchema.HasAdditionalProperties {
				// Both this schema, and the aggregate schema have additional
				// properties, they must match.
				if schema.AdditionalPropertiesType.TypeDecl() != outSchema.AdditionalPropertiesType.TypeDecl() {
					return Schema{}, errors.New("additional properties in allOf have incompatible types")
				}
			} else {
				// We're switching from having no additional properties to having
				// them
				outSchema.HasAdditionalProperties = true
				outSchema.AdditionalPropertiesType = schema.AdditionalPropertiesType
			}
		}
	}

	// Now, we generate the struct which merges together all the fields.
	var err error
	outSchema.GoType, err = GenStructFromAllOf(allOf, path)
	if err != nil {
		return Schema{}, errors.Wrap(err, "unable to generate aggregate type for AllOf")
	}
	return outSchema, nil
}

// This function generates an object that is the union of the objects in the
// input array. In the case of Ref objects, we use an embedded struct, otherwise,
// we inline the fields.
func GenStructFromAllOf(allOf []*openapi3.SchemaRef, path []string) (string, error) {
	// Start out with struct {
	objectParts := []string{"struct {"}
	for _, schemaOrRef := range allOf {
		ref := schemaOrRef.Ref
		if ref != "" {
			// We have a referenced type, we will generate an inlined struct
			// member.
			// struct {
			//   InlinedMember
			//   ...
			// }
			goType, err := RefPathToGoType(ref)
			if err != nil {
				return "", err
			}
			objectParts = append(objectParts,
				fmt.Sprintf("   // Embedded struct due to allOf(%s)", ref))
			objectParts = append(objectParts,
				fmt.Sprintf("   %s", goType))
		} else {
			// Inline all the fields from the schema into the output struct,
			// just like in the simple case of generating an object.
			goSchema, err := GenerateGoSchema(schemaOrRef, path)
			if err != nil {
				return "", err
			}
			objectParts = append(objectParts, "   // Embedded fields due to inline allOf schema")
			objectParts = append(objectParts, GenFieldsFromProperties(goSchema.Properties)...)

		}
	}
	objectParts = append(objectParts, "}")
	return strings.Join(objectParts, "\n"), nil
}

// MergeSchemasForEs do merge all the fields in the schemas supplied into one giant schema.
func MergeSchemasForEs(allOf []*openapi3.SchemaRef, path []string, tag string) (Schema, error) {
	var outSchema Schema
	// Now, we generate the struct which merges together all the fields.
	template, err := GenEsTemplateFromAllOf(allOf, path, tag)
	if err != nil {
		return Schema{}, errors.Wrap(err, "unable to generate aggregate indices for AllOf")
	}
	if template != "" {
		template = fmt.Sprintf(`"type": "nested",%s`, template)
	}
	outSchema.EsTemplate = template
	return outSchema, nil

}

// GenEsTemplateFromAllOf do create es template to for `allOf` field
func GenEsTemplateFromAllOf(allOf []*openapi3.SchemaRef, path []string, tag string) (string, error) {
	// Start out with "properties": {
	if tag != "" {
		return fmt.Sprintf(`"type": "%s"`, "nested"), nil
	}
	var props []string
	for _, schemaOrRef := range allOf {
		// Inline all the fields from the schema into the output struct,
		// just like in the simple case of generating an object.
		esSchema, err := GenerateEsSchema(schemaOrRef, path)
		if err != nil {
			return "", err
		}
		props = append(props, esSchema.EsTemplateDecl())
	}
	if strings.Join(props, "") == "" {
		return "", nil
	}

	return strings.Join(props, "\n"), nil
}

// GenEsTemplateFromReference do create es template from $ref field
// format will like `"properties": { "field1": { "type": "text" }, "field2": { "type": "text" } }`
func GenEsTemplateFromReference(reference *openapi3.SchemaRef, path []string) (string, error) {
	// fmt.Println(path, reference.Ref)
	s, err := isDeepMapObject(reference)
	if err != nil {
		return "", err
	}
	if s != "" {
		return s, nil
	}
	newRef := *reference
	newRef.Ref = ""
	// Inline all the fields from the schema into the output struct,
	// just like in the simple case of generating an object.
	esSchema, err := GenerateEsSchema(&newRef, path)
	if err != nil {
		return "", err
	}
	template := ""
	if esSchema.EsTemplateDecl() != "" {
		template = fmt.Sprintf(`"type": "nested",%s`, esSchema.EsTemplateDecl())
	}
	return template, nil
}

// This constructs a Go type for a parameter, looking at either the schema or
// the content, whichever is available
func paramToGoType(param *openapi3.Parameter, path []string) (Schema, error) {
	if param.Content == nil && param.Schema == nil {
		return Schema{}, fmt.Errorf("parameter '%s' has no schema or content", param.Name)
	}

	// We can process the schema through the generic schema processor
	if param.Schema != nil {
		return GenerateGoSchema(param.Schema, path)
	}

	// At this point, we have a content type. We know how to deal with
	// application/json, but if multiple formats are present, we can't do anything,
	// so we'll return the parameter as a string, not bothering to decode it.
	if len(param.Content) > 1 {
		return Schema{
			GoType: "string",
		}, nil
	}

	// Otherwise, look for application/json in there
	mt, found := param.Content["application/json"]
	if !found {
		// If we don't have json, it's a string
		return Schema{
			GoType: "string",
		}, nil
	}

	// For json, we go through the standard schema mechanism
	return GenerateGoSchema(mt.Schema, path)
}

func parseValidateRule(schema *openapi3.Schema, required bool) map[string]string {
	v := map[string]string{}

	if schema == nil {
		return v
	}

	if schema.MinLength > 0 {
		v["minlength"] = fmt.Sprintf("min=%d", schema.MinLength)
	}

	if schema.MaxLength != nil {
		v["maxlength"] = fmt.Sprintf("max=%d", *schema.MaxLength)
	}

	if schema.Min != nil {
		if schema.Type == "integer" {
			v["min"] = fmt.Sprintf("min=%d", int(*schema.Min))
		} else {
			v["min"] = fmt.Sprintf("min=%f", *schema.Min)
		}
	}
	if schema.Max != nil {
		if schema.Type == "integer" {
			v["max"] = fmt.Sprintf("max=%d", int(*schema.Max))
		} else {
			v["max"] = fmt.Sprintf("max=%f", *schema.Max)
		}
	}

	if schema.Pattern != "" {
		// This is deprecated as it may not work properly
		// https://github.com/go-playground/validator/issues/346
		v["regex"] = fmt.Sprintf("regex=%s", schema.Pattern)
	}

	// 	if schema.Format != "" {
	//		v["format"] = fmt.Sprintf("%s", schema.Format)
	//	}

	if schema.Type == "array" {
		// skip
		if schema.MinItems > 0 {
			v["minitems"] = fmt.Sprintf("min=%d", schema.MinItems)
		}

		if schema.MaxItems != nil {
			v["maxitems"] = fmt.Sprintf("max=%d", *schema.MaxItems)
		}
	}

	if len(schema.Enum) > 0 {
		if schema.Type != "array" {
			e := "oneof="
			for _, enum := range schema.Enum {
				e += fmt.Sprint(enum) + " "
			}
			v["oneof"] = e
		}
	}

	if len(schema.Extensions) > 0 {
		for key, ext := range schema.Extensions {
			if key == "x-go-custom-tag" {
				var s string
				var e []string
				err := json.Unmarshal(ext.(json.RawMessage), &s)
				if err != nil {
					if err := json.Unmarshal(ext.(json.RawMessage), &e); err != nil {
						panic(err)
					}
				} else {
					e = append(e, s)
				}
				v["custom-tag"] = strings.Join(e[:], ",")
			}
		}
	}
	return v
}

func parseEsType(schema *openapi3.Schema) string {
	v := ""
	if len(schema.Extensions) > 0 {
		for key, ext := range schema.Extensions {
			if key == "x-es-tag" {
				var s string
				var e []string
				err := json.Unmarshal(ext.(json.RawMessage), &s)
				if err != nil {
					if err := json.Unmarshal(ext.(json.RawMessage), &e); err != nil {
						panic(err)
					}
				} else {
					e = append(e, s)
				}
				v = strings.Join(e[:], ",")
			}
		}
	}
	return v
}

func isDeepMapObject(reference *openapi3.SchemaRef) (string, error) {
	fields := []string{}
	isCall := false
	isInfinityObject(reference, &fields, &isCall)
	if isCall {
		field := fmt.Sprintf(`"type": "%s"`, "nested")
		return field, nil
	}
	return "", nil
}

func isInfinityObject(ref *openapi3.SchemaRef, refs *[]string, isRecall *bool) {
	schema := ref.Value
	if ref.Ref != "" {
		*refs = append(*refs, ref.Ref)
	}
	dup := isDuplicate(*refs)
	*isRecall = dup
	if isRecall != nil && *isRecall {
		return
	}

	switch schema.Type {
	case "", "object":
		if len(schema.Properties) == 0 {
			return
		}
		// We've got an object with some properties.
		for _, pName := range SortedSchemaKeys(schema.Properties) {
			p := schema.Properties[pName]
			isInfinityObject(p, refs, isRecall)
		}
		if schema.AdditionalProperties != nil {
			isInfinityObject(schema.AdditionalProperties, refs, isRecall)
		}
	case "array":
		isInfinityObject(schema.Items, refs, isRecall)
	}
	return
}

func isDuplicate(refs []string) bool {
	// Use map to record duplicates as we find them.
	keys := map[string]struct{}{}

	for _, entry := range refs {
		_, ok := keys[entry]
		if !ok {
			keys[entry] = struct{}{}
		} else {
			return true
		}
	}
	return false
}
