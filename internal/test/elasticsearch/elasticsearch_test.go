package elasticsearch

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"github.com/indigonote/oapi-codegen/pkg/codegen"
)

const spec = `
openapi: 3.0.2
info:
  version: '0.0.1'
  title: example
  desscription: |
    Make sure that recursive types are handled properly
paths:
  /example:
    get:
      operationId: exampleGet
      responses:
        '200':
          description: "OK"
          content:
            'application/json':
              schema:
                $ref: '#/components/schemas/fhir-code-system'
components:
  schemas:
    fhir-code-system:
      title: fhir-code-system
      type: object
      properties:
        id:
          type: string
          x-go-custom-tag:
            - fhirID
          x-es-tag:
            - keyword
        status:
          type: string
          enum:
            - active
          default: active
          x-es-tag:
            - text
        concept:
          type: array
          items:
            $ref: '#/components/schemas/fhir-concept'
      x-tags:
        - elastic
    fhir-concept:
      title: fhir-concept
      type: object
      properties:
        code:
          type: string
          x-go-custom-tag:
            - fhirCode
          x-es-tag:
            - text
        display:
          type: string
          maxLength: 1048576
          x-go-custom-tag:
            - fhirString
          x-es-tag:
            - text
      required:
        - code
`

func TestElasticSearch(t *testing.T) {
	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData([]byte(spec))
	require.NoError(t, err)

	opts := codegen.Options{
		GenerateEsTemplate: true,
	}

	_, esCode, err := codegen.Generate(swagger, "elasticsearch", opts)
	require.NoError(t, err)
	require.NotEmpty(t, esCode)
	require.Equal(t, expect, esCode)

}

var expect = `{
    "FhirCodeSystem": {
        "mappings": {
            "properties": {
                "concept": {
                    "properties": {
                        "code": {
                            "fields": {
                              "keyword": {
                                  "ignore_above": 256,
                                  "type": "keyword"
                              }
                            },
                            "type": "text"
                        },
                        "display": {
                            "fields": {
                              "keyword": {
                                  "ignore_above": 256,
                                  "type": "keyword"
                              }
                            },
                            "type": "text"
                        }
                    },
                    "type": "nested"
                },
                "id": {
                    "type": "keyword"
                },
                "status": {
                    "fields": {
                      "keyword": {
                          "ignore_above": 256,
                          "type": "keyword"
                      }
                    },
                    "type": "text"
                }
            }
        }
    }
}`
