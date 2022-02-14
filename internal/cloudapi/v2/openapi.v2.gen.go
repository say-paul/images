// Package v2 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen version v1.8.2 DO NOT EDIT.
package v2

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
)

const (
	BearerScopes = "Bearer.Scopes"
)

// Defines values for ComposeStatusValue.
const (
	ComposeStatusValueFailure ComposeStatusValue = "failure"

	ComposeStatusValuePending ComposeStatusValue = "pending"

	ComposeStatusValueSuccess ComposeStatusValue = "success"
)

// Defines values for ImageStatusValue.
const (
	ImageStatusValueBuilding ImageStatusValue = "building"

	ImageStatusValueFailure ImageStatusValue = "failure"

	ImageStatusValuePending ImageStatusValue = "pending"

	ImageStatusValueRegistering ImageStatusValue = "registering"

	ImageStatusValueSuccess ImageStatusValue = "success"

	ImageStatusValueUploading ImageStatusValue = "uploading"
)

// Defines values for ImageTypes.
const (
	ImageTypesAws ImageTypes = "aws"

	ImageTypesAwsHaRhui ImageTypes = "aws-ha-rhui"

	ImageTypesAwsRhui ImageTypes = "aws-rhui"

	ImageTypesAwsSapRhui ImageTypes = "aws-sap-rhui"

	ImageTypesAzure ImageTypes = "azure"

	ImageTypesAzureRhui ImageTypes = "azure-rhui"

	ImageTypesEdgeCommit ImageTypes = "edge-commit"

	ImageTypesEdgeContainer ImageTypes = "edge-container"

	ImageTypesEdgeInstaller ImageTypes = "edge-installer"

	ImageTypesGcp ImageTypes = "gcp"

	ImageTypesGuestImage ImageTypes = "guest-image"

	ImageTypesImageInstaller ImageTypes = "image-installer"

	ImageTypesVsphere ImageTypes = "vsphere"
)

// Defines values for UploadStatusValue.
const (
	UploadStatusValueFailure UploadStatusValue = "failure"

	UploadStatusValuePending UploadStatusValue = "pending"

	UploadStatusValueRunning UploadStatusValue = "running"

	UploadStatusValueSuccess UploadStatusValue = "success"
)

// Defines values for UploadTypes.
const (
	UploadTypesAws UploadTypes = "aws"

	UploadTypesAwsS3 UploadTypes = "aws.s3"

	UploadTypesAzure UploadTypes = "azure"

	UploadTypesGcp UploadTypes = "gcp"
)

// AWSEC2UploadOptions defines model for AWSEC2UploadOptions.
type AWSEC2UploadOptions struct {
	Region            string   `json:"region"`
	ShareWithAccounts []string `json:"share_with_accounts"`
	SnapshotName      *string  `json:"snapshot_name,omitempty"`
}

// AWSEC2UploadStatus defines model for AWSEC2UploadStatus.
type AWSEC2UploadStatus struct {
	Ami    string `json:"ami"`
	Region string `json:"region"`
}

// AWSS3UploadOptions defines model for AWSS3UploadOptions.
type AWSS3UploadOptions struct {
	Region string `json:"region"`
}

// AWSS3UploadStatus defines model for AWSS3UploadStatus.
type AWSS3UploadStatus struct {
	Url string `json:"url"`
}

// AzureUploadOptions defines model for AzureUploadOptions.
type AzureUploadOptions struct {
	// Name of the uploaded image. It must be unique in the given resource group.
	// If name is omitted from the request, a random one based on a UUID is
	// generated.
	ImageName *string `json:"image_name,omitempty"`

	// Location where the image should be uploaded and registered.
	// How to list all locations:
	// https://docs.microsoft.com/en-us/cli/azure/account?view=azure-cli-latest#az_account_list_locations'
	Location string `json:"location"`

	// Name of the resource group where the image should be uploaded.
	ResourceGroup string `json:"resource_group"`

	// ID of subscription where the image should be uploaded.
	SubscriptionId string `json:"subscription_id"`

	// ID of the tenant where the image should be uploaded.
	// How to find it in the Azure Portal:
	// https://docs.microsoft.com/en-us/azure/active-directory/fundamentals/active-directory-how-to-find-tenant
	TenantId string `json:"tenant_id"`
}

// AzureUploadStatus defines model for AzureUploadStatus.
type AzureUploadStatus struct {
	ImageName string `json:"image_name"`
}

// ComposeId defines model for ComposeId.
type ComposeId struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	Id string `json:"id"`
}

// ComposeLogs defines model for ComposeLogs.
type ComposeLogs struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	ImageBuilds []interface{} `json:"image_builds"`
	Koji        *KojiLogs     `json:"koji,omitempty"`
}

// ComposeManifests defines model for ComposeManifests.
type ComposeManifests struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	Manifests []interface{} `json:"manifests"`
}

// ComposeMetadata defines model for ComposeMetadata.
type ComposeMetadata struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	// ID (hash) of the built commit
	OstreeCommit *string `json:"ostree_commit,omitempty"`

	// Package list including NEVRA
	Packages *[]PackageMetadata `json:"packages,omitempty"`
}

// ComposeRequest defines model for ComposeRequest.
type ComposeRequest struct {
	Customizations *Customizations `json:"customizations,omitempty"`
	Distribution   string          `json:"distribution"`
	ImageRequest   *ImageRequest   `json:"image_request,omitempty"`
	ImageRequests  *[]ImageRequest `json:"image_requests,omitempty"`
	Koji           *Koji           `json:"koji,omitempty"`
}

// ComposeStatus defines model for ComposeStatus.
type ComposeStatus struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	ImageStatus   ImageStatus        `json:"image_status"`
	ImageStatuses *[]ImageStatus     `json:"image_statuses,omitempty"`
	KojiStatus    *KojiStatus        `json:"koji_status,omitempty"`
	Status        ComposeStatusValue `json:"status"`
}

// ComposeStatusError defines model for ComposeStatusError.
type ComposeStatusError struct {
	Details *interface{} `json:"details,omitempty"`
	Id      int          `json:"id"`
	Reason  string       `json:"reason"`
}

// ComposeStatusValue defines model for ComposeStatusValue.
type ComposeStatusValue string

// Customizations defines model for Customizations.
type Customizations struct {
	Filesystem *[]Filesystem `json:"filesystem,omitempty"`
	Packages   *[]string     `json:"packages,omitempty"`

	// Extra repositories for packages specified in customizations. These
	// repositories will only be used to depsolve and retrieve packages
	// for the OS itself (they will not be available for the build root or
	// any other part of the build process). The package_sets field for these
	// repositories is ignored.
	PayloadRepositories *[]Repository `json:"payload_repositories,omitempty"`
	Subscription        *Subscription `json:"subscription,omitempty"`
	Users               *[]User       `json:"users,omitempty"`
}

// Error defines model for Error.
type Error struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	Code        string `json:"code"`
	OperationId string `json:"operation_id"`
	Reason      string `json:"reason"`
}

// ErrorList defines model for ErrorList.
type ErrorList struct {
	// Embedded struct due to allOf(#/components/schemas/List)
	List `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	Items []Error `json:"items"`
}

// Filesystem defines model for Filesystem.
type Filesystem struct {
	MinSize    int    `json:"min_size"`
	Mountpoint string `json:"mountpoint"`
}

// GCPUploadOptions defines model for GCPUploadOptions.
type GCPUploadOptions struct {
	// Name of an existing STANDARD Storage class Bucket.
	Bucket string `json:"bucket"`

	// The name to use for the imported and shared Compute Engine image.
	// The image name must be unique within the GCP project, which is used
	// for the OS image upload and import. If not specified a random
	// 'composer-api-<uuid>' string is used as the image name.
	ImageName *string `json:"image_name,omitempty"`

	// The GCP region where the OS image will be imported to and shared from.
	// The value must be a valid GCP location. See https://cloud.google.com/storage/docs/locations.
	// If not specified, the multi-region location closest to the source
	// (source Storage Bucket location) is chosen automatically.
	Region string `json:"region"`

	// List of valid Google accounts to share the imported Compute Engine image with.
	// Each string must contain a specifier of the account type. Valid formats are:
	//   - 'user:{emailid}': An email address that represents a specific
	//     Google account. For example, 'alice@example.com'.
	//   - 'serviceAccount:{emailid}': An email address that represents a
	//     service account. For example, 'my-other-app@appspot.gserviceaccount.com'.
	//   - 'group:{emailid}': An email address that represents a Google group.
	//     For example, 'admins@example.com'.
	//   - 'domain:{domain}': The G Suite domain (primary) that represents all
	//     the users of that domain. For example, 'google.com' or 'example.com'.
	// If not specified, the imported Compute Engine image is not shared with any
	// account.
	ShareWithAccounts *[]string `json:"share_with_accounts,omitempty"`
}

// GCPUploadStatus defines model for GCPUploadStatus.
type GCPUploadStatus struct {
	ImageName string `json:"image_name"`
	ProjectId string `json:"project_id"`
}

// ImageRequest defines model for ImageRequest.
type ImageRequest struct {
	Architecture  string         `json:"architecture"`
	ImageType     ImageTypes     `json:"image_type"`
	Ostree        *OSTree        `json:"ostree,omitempty"`
	Repositories  []Repository   `json:"repositories"`
	UploadOptions *UploadOptions `json:"upload_options,omitempty"`
}

// ImageStatus defines model for ImageStatus.
type ImageStatus struct {
	Error        *ComposeStatusError `json:"error,omitempty"`
	Status       ImageStatusValue    `json:"status"`
	UploadStatus *UploadStatus       `json:"upload_status,omitempty"`
}

// ImageStatusValue defines model for ImageStatusValue.
type ImageStatusValue string

// ImageTypes defines model for ImageTypes.
type ImageTypes string

// Koji defines model for Koji.
type Koji struct {
	Name    string `json:"name"`
	Release string `json:"release"`
	Server  string `json:"server"`
	TaskId  int    `json:"task_id"`
	Version string `json:"version"`
}

// KojiLogs defines model for KojiLogs.
type KojiLogs struct {
	Import interface{} `json:"import"`
	Init   interface{} `json:"init"`
}

// KojiStatus defines model for KojiStatus.
type KojiStatus struct {
	BuildId *int `json:"build_id,omitempty"`
}

// List defines model for List.
type List struct {
	Kind  string `json:"kind"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
	Total int    `json:"total"`
}

// OSTree defines model for OSTree.
type OSTree struct {
	Parent *string `json:"parent,omitempty"`
	Ref    *string `json:"ref,omitempty"`
	Url    *string `json:"url,omitempty"`
}

// ObjectReference defines model for ObjectReference.
type ObjectReference struct {
	Href string `json:"href"`
	Id   string `json:"id"`
	Kind string `json:"kind"`
}

// PackageMetadata defines model for PackageMetadata.
type PackageMetadata struct {
	Arch      string  `json:"arch"`
	Epoch     *string `json:"epoch,omitempty"`
	Name      string  `json:"name"`
	Release   string  `json:"release"`
	Sigmd5    string  `json:"sigmd5"`
	Signature *string `json:"signature,omitempty"`
	Type      string  `json:"type"`
	Version   string  `json:"version"`
}

// Repository defines model for Repository.
type Repository struct {
	Baseurl    *string `json:"baseurl,omitempty"`
	CheckGpg   *bool   `json:"check_gpg,omitempty"`
	GpgKey     *string `json:"gpg_key,omitempty"`
	IgnoreSsl  *bool   `json:"ignore_ssl,omitempty"`
	Metalink   *string `json:"metalink,omitempty"`
	Mirrorlist *string `json:"mirrorlist,omitempty"`

	// Naming package sets for a repository assigns it to a specific part
	// (pipeline) of the build process.
	PackageSets *[]string `json:"package_sets,omitempty"`
	Rhsm        bool      `json:"rhsm"`
}

// Subscription defines model for Subscription.
type Subscription struct {
	ActivationKey string `json:"activation_key"`
	BaseUrl       string `json:"base_url"`
	Insights      bool   `json:"insights"`
	Organization  string `json:"organization"`
	ServerUrl     string `json:"server_url"`
}

// UploadOptions defines model for UploadOptions.
type UploadOptions interface{}

// UploadStatus defines model for UploadStatus.
type UploadStatus struct {
	Options interface{}       `json:"options"`
	Status  UploadStatusValue `json:"status"`
	Type    UploadTypes       `json:"type"`
}

// UploadStatusValue defines model for UploadStatusValue.
type UploadStatusValue string

// UploadTypes defines model for UploadTypes.
type UploadTypes string

// User defines model for User.
type User struct {
	// Embedded struct due to allOf(#/components/schemas/ObjectReference)
	ObjectReference `yaml:",inline"`
	// Embedded fields due to inline allOf schema
	Groups *[]string `json:"groups,omitempty"`
	Key    *string   `json:"key,omitempty"`
	Name   string    `json:"name"`
}

// Page defines model for page.
type Page string

// Size defines model for size.
type Size string

// PostComposeJSONBody defines parameters for PostCompose.
type PostComposeJSONBody ComposeRequest

// GetErrorListParams defines parameters for GetErrorList.
type GetErrorListParams struct {
	// Page index
	Page *Page `json:"page,omitempty"`

	// Number of items in each page
	Size *Size `json:"size,omitempty"`
}

// PostComposeJSONRequestBody defines body for PostCompose for application/json ContentType.
type PostComposeJSONRequestBody PostComposeJSONBody

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Create compose
	// (POST /compose)
	PostCompose(ctx echo.Context) error
	// The status of a compose
	// (GET /composes/{id})
	GetComposeStatus(ctx echo.Context, id string) error
	// Get logs for a compose.
	// (GET /composes/{id}/logs)
	GetComposeLogs(ctx echo.Context, id string) error
	// Get the manifests for a compose.
	// (GET /composes/{id}/manifests)
	GetComposeManifests(ctx echo.Context, id string) error
	// Get the metadata for a compose.
	// (GET /composes/{id}/metadata)
	GetComposeMetadata(ctx echo.Context, id string) error
	// Get a list of all possible errors
	// (GET /errors)
	GetErrorList(ctx echo.Context, params GetErrorListParams) error
	// Get error description
	// (GET /errors/{id})
	GetError(ctx echo.Context, id string) error
	// Get the openapi spec in json format
	// (GET /openapi)
	GetOpenapi(ctx echo.Context) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// PostCompose converts echo context to params.
func (w *ServerInterfaceWrapper) PostCompose(ctx echo.Context) error {
	var err error

	ctx.Set(BearerScopes, []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.PostCompose(ctx)
	return err
}

// GetComposeStatus converts echo context to params.
func (w *ServerInterfaceWrapper) GetComposeStatus(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id string

	err = runtime.BindStyledParameterWithLocation("simple", false, "id", runtime.ParamLocationPath, ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	ctx.Set(BearerScopes, []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetComposeStatus(ctx, id)
	return err
}

// GetComposeLogs converts echo context to params.
func (w *ServerInterfaceWrapper) GetComposeLogs(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id string

	err = runtime.BindStyledParameterWithLocation("simple", false, "id", runtime.ParamLocationPath, ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetComposeLogs(ctx, id)
	return err
}

// GetComposeManifests converts echo context to params.
func (w *ServerInterfaceWrapper) GetComposeManifests(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id string

	err = runtime.BindStyledParameterWithLocation("simple", false, "id", runtime.ParamLocationPath, ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetComposeManifests(ctx, id)
	return err
}

// GetComposeMetadata converts echo context to params.
func (w *ServerInterfaceWrapper) GetComposeMetadata(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id string

	err = runtime.BindStyledParameterWithLocation("simple", false, "id", runtime.ParamLocationPath, ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	ctx.Set(BearerScopes, []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetComposeMetadata(ctx, id)
	return err
}

// GetErrorList converts echo context to params.
func (w *ServerInterfaceWrapper) GetErrorList(ctx echo.Context) error {
	var err error

	ctx.Set(BearerScopes, []string{""})

	// Parameter object where we will unmarshal all parameters from the context
	var params GetErrorListParams
	// ------------- Optional query parameter "page" -------------

	err = runtime.BindQueryParameter("form", true, false, "page", ctx.QueryParams(), &params.Page)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter page: %s", err))
	}

	// ------------- Optional query parameter "size" -------------

	err = runtime.BindQueryParameter("form", true, false, "size", ctx.QueryParams(), &params.Size)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter size: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetErrorList(ctx, params)
	return err
}

// GetError converts echo context to params.
func (w *ServerInterfaceWrapper) GetError(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "id" -------------
	var id string

	err = runtime.BindStyledParameterWithLocation("simple", false, "id", runtime.ParamLocationPath, ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	ctx.Set(BearerScopes, []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetError(ctx, id)
	return err
}

// GetOpenapi converts echo context to params.
func (w *ServerInterfaceWrapper) GetOpenapi(ctx echo.Context) error {
	var err error

	ctx.Set(BearerScopes, []string{""})

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetOpenapi(ctx)
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
	RegisterHandlersWithBaseURL(router, si, "")
}

// Registers handlers, and prepends BaseURL to the paths, so that the paths
// can be served under a prefix.
func RegisterHandlersWithBaseURL(router EchoRouter, si ServerInterface, baseURL string) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.POST(baseURL+"/compose", wrapper.PostCompose)
	router.GET(baseURL+"/composes/:id", wrapper.GetComposeStatus)
	router.GET(baseURL+"/composes/:id/logs", wrapper.GetComposeLogs)
	router.GET(baseURL+"/composes/:id/manifests", wrapper.GetComposeManifests)
	router.GET(baseURL+"/composes/:id/metadata", wrapper.GetComposeMetadata)
	router.GET(baseURL+"/errors", wrapper.GetErrorList)
	router.GET(baseURL+"/errors/:id", wrapper.GetError)
	router.GET(baseURL+"/openapi", wrapper.GetOpenapi)

}

// Base64 encoded, gzipped, json marshaled Swagger object
var swaggerSpec = []string{

	"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",
	"D7LRSqTlE6SKzTY6Nw//FwAA///Ls+uMj1AAAA==",
}

// GetSwagger returns the content of the embedded swagger specification file
// or error if failed to decode
func decodeSpec() ([]byte, error) {
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

	return buf.Bytes(), nil
}

var rawSpec = decodeSpecCached()

// a naive cached of a decoded swagger spec
func decodeSpecCached() func() ([]byte, error) {
	data, err := decodeSpec()
	return func() ([]byte, error) {
		return data, err
	}
}

// Constructs a synthetic filesystem for resolving external references when loading openapi specifications.
func PathToRawSpec(pathToFile string) map[string]func() ([]byte, error) {
	var res = make(map[string]func() ([]byte, error))
	if len(pathToFile) > 0 {
		res[pathToFile] = rawSpec
	}

	return res
}

// GetSwagger returns the Swagger specification corresponding to the generated code
// in this file. The external references of Swagger specification are resolved.
// The logic of resolving external references is tightly connected to "import-mapping" feature.
// Externally referenced files must be embedded in the corresponding golang packages.
// Urls can be supported but this task was out of the scope.
func GetSwagger() (swagger *openapi3.T, err error) {
	var resolvePath = PathToRawSpec("")

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(loader *openapi3.Loader, url *url.URL) ([]byte, error) {
		var pathToFile = url.String()
		pathToFile = path.Clean(pathToFile)
		getSpec, ok := resolvePath[pathToFile]
		if !ok {
			err1 := fmt.Errorf("path not found: %s", pathToFile)
			return nil, err1
		}
		return getSpec()
	}
	var specData []byte
	specData, err = rawSpec()
	if err != nil {
		return
	}
	swagger, err = loader.LoadFromData(specData)
	if err != nil {
		return
	}
	return
}
