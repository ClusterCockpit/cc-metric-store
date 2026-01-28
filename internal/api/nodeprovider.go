// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// BackendNodeProvider implements metricstore.NodeProvider by querying
// the cc-backend /api/jobs/used_nodes endpoint.
type BackendNodeProvider struct {
	backendUrl string
	client     *http.Client
}

// NewBackendNodeProvider creates a new BackendNodeProvider that queries
// the given cc-backend URL for used nodes information.
func NewBackendNodeProvider(backendUrl string) *BackendNodeProvider {
	return &BackendNodeProvider{
		backendUrl: backendUrl,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetUsedNodes returns a map of cluster names to sorted lists of unique hostnames
// that are currently in use by jobs that started before the given timestamp.
func (p *BackendNodeProvider) GetUsedNodes(ts int64) (map[string][]string, error) {
	url := fmt.Sprintf("%s/api/jobs/used_nodes?ts=%d", p.backendUrl, ts)

	resp, err := p.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("querying used nodes from backend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status %d", resp.StatusCode)
	}

	var result map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding used nodes response: %w", err)
	}

	return result, nil
}
