package swagger

// Route describes a single API operation for the generated OpenAPI spec.
type Route struct {
	Method      string
	Path        string
	Tag         string
	Summary     string
	Description string
	Params      []Param
	Body        *BodySpec
	Responses   map[int]ResponseSpec
}

// Param describes an operation parameter (path, query, header).
type Param struct {
	Name        string
	In          string // "path", "query", "header"
	Required    bool
	Description string
	Schema      map[string]interface{}
}

// BodySpec describes a request body.
type BodySpec struct {
	Required    bool
	Description string
	ContentType string
	Schema      map[string]interface{}
}

// ResponseSpec describes an HTTP response.
type ResponseSpec struct {
	Description string
	ContentType string
	Schema      map[string]interface{}
}

// Routes returns the canonical API route annotations for the pastebin service.
// These are the authoritative source from which the OpenAPI JSON spec is generated.
func Routes() []Route {
	pasteSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":              map[string]interface{}{"type": "string", "example": "AbCdEfGh"},
			"title":           map[string]interface{}{"type": "string", "example": "My Paste"},
			"content":         map[string]interface{}{"type": "string", "example": "hello world"},
			"language":        map[string]interface{}{"type": "string", "example": "go"},
			"is_public":       map[string]interface{}{"type": "boolean", "example": true},
			"created_at":      map[string]interface{}{"type": "string", "format": "date-time"},
			"expires_at":      map[string]interface{}{"type": "string", "format": "date-time", "nullable": true},
			"burn_after_read": map[string]interface{}{"type": "integer", "example": 0},
			"view_count":      map[string]interface{}{"type": "integer", "example": 42},
		},
	}

	pasteListSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pastes": map[string]interface{}{
				"type":  "array",
				"items": pasteSchema,
			},
			"total": map[string]interface{}{"type": "integer", "example": 100},
			"page":  map[string]interface{}{"type": "integer", "example": 1},
			"limit": map[string]interface{}{"type": "integer", "example": 20},
		},
	}

	createSchema := map[string]interface{}{
		"type":     "object",
		"required": []string{"content"},
		"properties": map[string]interface{}{
			"content":    map[string]interface{}{"type": "string", "description": "Paste body text"},
			"title":      map[string]interface{}{"type": "string", "description": "Optional title"},
			"language":   map[string]interface{}{"type": "string", "description": "Chroma language identifier; omit for auto-detect"},
			"is_public":  map[string]interface{}{"type": "boolean", "default": true, "description": "false = unlisted"},
			"expires_in": map[string]interface{}{"type": "string", "example": "1d", "description": "Duration: 1h 1d 1w 1m 3m 6m 1y 18m 2y never"},
			"burn_after": map[string]interface{}{"type": "integer", "default": 0, "description": "Delete after N views (0 = disabled; max 9999)"},
		},
	}

	errSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"error": map[string]interface{}{"type": "string"},
		},
	}

	return []Route{
		{
			Method:      "GET",
			Path:        "/api/v1/pastes",
			Tag:         "pastes",
			Summary:     "List public pastes",
			Description: "Returns a paginated list of recent public pastes.",
			Params: []Param{
				{Name: "page", In: "query", Required: false, Description: "Page number (default 1)", Schema: map[string]interface{}{"type": "integer", "default": 1}},
				{Name: "limit", In: "query", Required: false, Description: "Items per page (1–100, default 20)", Schema: map[string]interface{}{"type": "integer", "default": 20}},
			},
			Responses: map[int]ResponseSpec{
				200: {Description: "OK", ContentType: "application/json", Schema: pasteListSchema},
				500: {Description: "Internal server error", ContentType: "application/json", Schema: errSchema},
			},
		},
		{
			Method:      "POST",
			Path:        "/api/v1/pastes",
			Tag:         "pastes",
			Summary:     "Create paste",
			Description: "Creates a new paste. Accepts JSON or multipart/form-data.",
			Body: &BodySpec{
				Required:    true,
				Description: "Paste creation request",
				ContentType: "application/json",
				Schema:      createSchema,
			},
			Responses: map[int]ResponseSpec{
				201: {Description: "Paste created", ContentType: "application/json", Schema: pasteSchema},
				400: {Description: "Bad request", ContentType: "application/json", Schema: errSchema},
				429: {Description: "Rate limit exceeded", ContentType: "application/json", Schema: errSchema},
			},
		},
		{
			Method:      "GET",
			Path:        "/api/v1/pastes/{id}",
			Tag:         "pastes",
			Summary:     "Get paste",
			Description: "Returns paste metadata and content by ID.",
			Params: []Param{
				{Name: "id", In: "path", Required: true, Description: "Paste identifier", Schema: map[string]interface{}{"type": "string"}},
			},
			Responses: map[int]ResponseSpec{
				200: {Description: "OK", ContentType: "application/json", Schema: pasteSchema},
				404: {Description: "Paste not found", ContentType: "application/json", Schema: errSchema},
			},
		},
		{
			Method:      "DELETE",
			Path:        "/api/v1/pastes/{id}",
			Tag:         "pastes",
			Summary:     "Delete paste",
			Description: "Deletes a paste. Requires `Authorization: Bearer <token>` header or `?token=` query param (the delete token returned at creation time).",
			Params: []Param{
				{Name: "id", In: "path", Required: true, Description: "Paste identifier", Schema: map[string]interface{}{"type": "string"}},
				{Name: "token", In: "query", Required: false, Description: "Delete token (alternative to Authorization header)", Schema: map[string]interface{}{"type": "string"}},
			},
			Responses: map[int]ResponseSpec{
				204: {Description: "Deleted"},
				401: {Description: "Unauthorized", ContentType: "application/json", Schema: errSchema},
				404: {Description: "Paste not found", ContentType: "application/json", Schema: errSchema},
			},
		},
		{
			Method:      "GET",
			Path:        "/api/v1/pastes/{id}/raw",
			Tag:         "pastes",
			Summary:     "Get raw paste content",
			Description: "Returns the raw paste text as plain text.",
			Params: []Param{
				{Name: "id", In: "path", Required: true, Description: "Paste identifier", Schema: map[string]interface{}{"type": "string"}},
			},
			Responses: map[int]ResponseSpec{
				200: {Description: "Raw paste text", ContentType: "text/plain"},
				404: {Description: "Paste not found", ContentType: "text/plain"},
			},
		},
		{
			Method:      "GET",
			Path:        "/api/v1/server/healthz",
			Tag:         "server",
			Summary:     "Health check",
			Description: "Returns server health status and basic metrics.",
			Responses: map[int]ResponseSpec{
				200: {
					Description: "OK",
					ContentType: "application/json",
					Schema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"status":  map[string]interface{}{"type": "string", "example": "ok"},
							"version": map[string]interface{}{"type": "string"},
							"uptime":  map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
		{
			Method:      "GET",
			Path:        "/api/v1/server/version",
			Tag:         "server",
			Summary:     "Version info",
			Description: "Returns the server version, commit ID, and build date.",
			Responses: map[int]ResponseSpec{
				200: {
					Description: "OK",
					ContentType: "application/json",
					Schema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"version":    map[string]interface{}{"type": "string"},
							"commit":     map[string]interface{}{"type": "string"},
							"build_date": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
		{
			Method:      "GET",
			Path:        "/api/v1/server/swagger",
			Tag:         "server",
			Summary:     "OpenAPI specification",
			Description: "Returns the OpenAPI 3.0.3 JSON specification for this API.",
			Responses: map[int]ResponseSpec{
				200: {Description: "OpenAPI JSON spec", ContentType: "application/json"},
			},
		},
		{
			Method:      "POST",
			Path:        "/api/v1/server/graphql",
			Tag:         "server",
			Summary:     "GraphQL endpoint",
			Description: "Executes a GraphQL query or mutation. The interactive GraphiQL explorer is available at /server/docs/graphql.",
			Body: &BodySpec{
				Required:    true,
				ContentType: "application/json",
				Description: "GraphQL request with query and optional variables.",
				Schema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query":     map[string]interface{}{"type": "string"},
						"variables": map[string]interface{}{"type": "object"},
					},
					"required": []string{"query"},
				},
			},
			Responses: map[int]ResponseSpec{
				200: {
					Description: "GraphQL response",
					ContentType: "application/json",
					Schema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"data":   map[string]interface{}{"type": "object"},
							"errors": map[string]interface{}{"type": "array"},
						},
					},
				},
			},
		},
	}
}
