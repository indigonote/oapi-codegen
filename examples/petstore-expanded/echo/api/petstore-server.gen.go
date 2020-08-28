// Package api provides primitives to interact the openapi HTTP API.
//
// Code generated by github.com/indigonote/oapi-codegen DO NOT EDIT.
package api

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Returns all pets
	// (GET /pets)
	FindPets(ctx echo.Context, params FindPetsParams) error
	// Creates a new pet
	// (POST /pets)
	AddPet(ctx echo.Context) error
	// Deletes a pet by ID
	// (DELETE /pets/{id})
	DeletePet(ctx echo.Context, id int64) error
	// Returns a pet by ID
	// (GET /pets/{id})
	FindPetById(ctx echo.Context, id int64) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// FindPets converts echo context to params.
func (w *ServerInterfaceWrapper) FindPets(ctx echo.Context) error {
	var err error

	// Parameter object where we will unmarshal all parameters from the context
	var params FindPetsParams
	// ------------- Optional query parameter "tags" -------------

	err = runtime.BindQueryParameter("form", true, false, "tags", ctx.QueryParams(), &params.Tags)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter tags: %s", err))
	}

	// ------------- Optional query parameter "limit" -------------

	err = runtime.BindQueryParameter("form", true, false, "limit", ctx.QueryParams(), &params.Limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter limit: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.FindPets(ctx, params)
	return err
}

// AddPet converts echo context to params.
func (w *ServerInterfaceWrapper) AddPet(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.AddPet(ctx)
	return err
}

// DeletePet converts echo context to params.
func (w *ServerInterfaceWrapper) DeletePet(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id int64

	err = runtime.BindStyledParameter("simple", false, "id", ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.DeletePet(ctx, id)
	return err
}

// FindPetById converts echo context to params.
func (w *ServerInterfaceWrapper) FindPetById(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id int64

	err = runtime.BindStyledParameter("simple", false, "id", ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.FindPetById(ctx, id)
	return err
}

// This is a simple interface which specifies echo.Route addition functions which
// are present on both echo.Echo and echo.Group, since we want to allow using
// either of them for path registration
type EchoRouter interface {
	CONNECT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	HEAD(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	OPTIONS(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	TRACE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router EchoRouter, si ServerInterface) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET("/pets", wrapper.FindPets)
	router.POST("/pets", wrapper.AddPet)
	router.DELETE("/pets/:id", wrapper.DeletePet)
	router.GET("/pets/:id", wrapper.FindPetById)

}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
	"AAD///YSrNU1EwAA",
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file.
func GetSwagger() (*openapi3.Swagger, error) {
	zipped, err := base64.StdEncoding.DecodeString(strings.Join(swaggerSpec, ""))
	if err != nil {
		return nil, fmt.Errorf("error base64 decoding spec: %s", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(zr)
	if err != nil {
		return nil, fmt.Errorf("error decompressing spec: %s", err)
	}

	swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error loading Swagger: %s", err)
	}
	return swagger, nil
}
