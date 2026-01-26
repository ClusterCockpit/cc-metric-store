// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-metric-store.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v4"
)

func authHandler(next http.Handler, publicKey ed25519.PublicKey) http.Handler {
	cacheLock := sync.RWMutex{}
	cache := map[string]*jwt.Token{}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		authheader := r.Header.Get("Authorization")
		if authheader == "" || !strings.HasPrefix(authheader, "Bearer ") {
			http.Error(rw, "Use JWT Authentication", http.StatusUnauthorized)
			return
		}

		rawtoken := authheader[len("Bearer "):]
		cacheLock.RLock()
		token, ok := cache[rawtoken]
		cacheLock.RUnlock()
		if ok && token.Claims.Valid() == nil {
			next.ServeHTTP(rw, r)
			return
		}

		// The actual token is ignored for now.
		// In case expiration and so on are specified, the Parse function
		// already returns an error for expired tokens.
		var err error
		token, err = jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
			if t.Method != jwt.SigningMethodEdDSA {
				return nil, errors.New("only Ed25519/EdDSA supported")
			}

			return publicKey, nil
		})
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		cacheLock.Lock()
		cache[rawtoken] = token
		cacheLock.Unlock()

		// Let request through...
		next.ServeHTTP(rw, r)
	})
}
