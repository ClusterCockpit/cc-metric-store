// Package api Code generated by swaggo/swag. DO NOT EDIT
package api

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {
            "name": "ClusterCockpit Project",
            "url": "https://clustercockpit.org",
            "email": "support@clustercockpit.org"
        },
        "license": {
            "name": "MIT License",
            "url": "https://opensource.org/licenses/MIT"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/debug/": {
            "post": {
                "security": [
                    {
                        "ApiKeyAuth": []
                    }
                ],
                "description": "Write metrics to store",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "debug"
                ],
                "summary": "Debug endpoint",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Selector",
                        "name": "selector",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Debug dump",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/free/": {
            "post": {
                "security": [
                    {
                        "ApiKeyAuth": []
                    }
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "free"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "description": "up to timestamp",
                        "name": "to",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "ok",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/query/": {
            "get": {
                "security": [
                    {
                        "ApiKeyAuth": []
                    }
                ],
                "description": "Query metrics.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "query"
                ],
                "summary": "Query metrics",
                "parameters": [
                    {
                        "description": "API query payload object",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.ApiQueryRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "API query response object",
                        "schema": {
                            "$ref": "#/definitions/api.ApiQueryResponse"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/write/": {
            "post": {
                "security": [
                    {
                        "ApiKeyAuth": []
                    }
                ],
                "consumes": [
                    "text/plain"
                ],
                "produces": [
                    "application/json"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "description": "If the lines in the body do not have a cluster tag, use this value instead.",
                        "name": "cluster",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "ok",
                        "schema": {
                            "type": "string"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "Unauthorized",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "Forbidden",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "api.ApiMetricData": {
            "type": "object",
            "properties": {
                "avg": {
                    "type": "number"
                },
                "data": {
                    "type": "array",
                    "items": {
                        "type": "number"
                    }
                },
                "error": {
                    "type": "string"
                },
                "from": {
                    "type": "integer"
                },
                "max": {
                    "type": "number"
                },
                "min": {
                    "type": "number"
                },
                "resolution": {
                    "type": "integer"
                },
                "to": {
                    "type": "integer"
                }
            }
        },
        "api.ApiQuery": {
            "type": "object",
            "properties": {
                "aggreg": {
                    "type": "boolean"
                },
                "host": {
                    "type": "string"
                },
                "metric": {
                    "type": "string"
                },
                "resolution": {
                    "type": "integer"
                },
                "scale-by": {
                    "type": "number"
                },
                "subtype": {
                    "type": "string"
                },
                "subtype-ids": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "type": {
                    "type": "string"
                },
                "type-ids": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        },
        "api.ApiQueryRequest": {
            "type": "object",
            "properties": {
                "cluster": {
                    "type": "string"
                },
                "for-all-nodes": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "from": {
                    "type": "integer"
                },
                "queries": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/api.ApiQuery"
                    }
                },
                "to": {
                    "type": "integer"
                },
                "with-data": {
                    "type": "boolean"
                },
                "with-padding": {
                    "type": "boolean"
                },
                "with-stats": {
                    "type": "boolean"
                }
            }
        },
        "api.ApiQueryResponse": {
            "type": "object",
            "properties": {
                "queries": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/api.ApiQuery"
                    }
                },
                "results": {
                    "type": "array",
                    "items": {
                        "type": "array",
                        "items": {
                            "$ref": "#/definitions/api.ApiMetricData"
                        }
                    }
                }
            }
        },
        "api.ErrorResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "description": "Error Message",
                    "type": "string"
                },
                "status": {
                    "description": "Statustext of Errorcode",
                    "type": "string"
                }
            }
        }
    },
    "securityDefinitions": {
        "ApiKeyAuth": {
            "type": "apiKey",
            "name": "X-Auth-Token",
            "in": "header"
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0.0",
	Host:             "localhost:8082",
	BasePath:         "/api/",
	Schemes:          []string{},
	Title:            "cc-metric-store REST API",
	Description:      "API for cc-metric-store",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}