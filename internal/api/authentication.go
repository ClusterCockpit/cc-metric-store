package api

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v4"
)

var publicKey ed25519.PublicKey

func isAuthenticated(r *http.Request) error {
	cacheLock := sync.RWMutex{}
	cache := map[string]*jwt.Token{}

	authheader := r.Header.Get("Authorization")
	if authheader == "" || !strings.HasPrefix(authheader, "Bearer ") {
		return errors.New("Use JWT Authentication")
	}

	rawtoken := authheader[len("Bearer "):]
	cacheLock.RLock()
	token, ok := cache[rawtoken]
	cacheLock.RUnlock()
	if ok && token.Claims.Valid() == nil {
		return nil
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
		return err
	}

	cacheLock.Lock()
	cache[rawtoken] = token
	cacheLock.Unlock()

	return nil
}
