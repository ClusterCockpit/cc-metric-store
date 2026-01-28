// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

var configSchema = `
{
  "type": "object",
  "properties": {
    "addr": {
      "description": "Address where the http (or https) server will listen on (for example: 'localhost:8080').",
      "type": "string"
    },
    "https-cert-file": {
      "description": "Filepath to SSL certificate. If also https-key-file is set, use HTTPS.",
      "type": "string"
    },
    "https-key-file": {
      "description": "Filepath to SSL key file. If also https-cert-file is set, use HTTPS.",
      "type": "string"
    },
    "user": {
      "description": "Drop root permissions once the port was taken. Only applicable if using privileged port.",
      "type": "string"
    },
    "group": {
      "description": "Drop root permissions once the port was taken. Only applicable if using privileged port.",
      "type": "string"
    },
    "debug": {
      "description": "Debug options.",
      "type": "object",
      "properties": {
        "dump-to-file": {
          "description": "Path to file for dumping internal state.",
          "type": "string"
        },
        "gops": {
          "description": "Enable gops agent for debugging.",
          "type": "boolean"
        }
      }
    },
    "jwt-public-key": {
      "description": "Ed25519 public key for JWT verification.",
      "type": "string"
    }
  }
}`
