package websocket

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func TestClientSession_CanAccess(t *testing.T) {
	testCases := []struct {
		name          string
		claims        *CustomClaims
		action        string
		channel       string
		expected      bool
		authIsEnabled bool
	}{
		{
			name:   "Auth Disabled - Should always allow",
			claims: nil, // claims are nil when auth is off
			action: "publish",
			channel: "any.channel",
			expected: true,
			authIsEnabled: false,
		},
		{
			name: "Exact Match - Allowed",
			claims: &CustomClaims{
				Scopes: []string{"publish:user.updates"},
			},
			action:   "publish",
			channel:  "user.updates",
			expected: true,
			authIsEnabled: true,
		},
		{
			name: "Exact Match - Denied (Wrong Action)",
			claims: &CustomClaims{
				Scopes: []string{"subscribe:user.updates"},
			},
			action:   "publish",
			channel:  "user.updates",
			expected: false,
			authIsEnabled: true,
		},
		{
			name: "Wildcard Match - Allowed",
			claims: &CustomClaims{
				Scopes: []string{"publish:user.*"},
			},
			action:   "publish",
			channel:  "user.12345",
			expected: true,
			authIsEnabled: true,
		},
		{
			name: "Wildcard Match - Denied (Wrong Prefix)",
			claims: &CustomClaims{
				Scopes: []string{"publish:admin.*"},
			},
			action:   "publish",
			channel:  "user.12345",
			expected: false,
			authIsEnabled: true,
		},
		{
			name: "No Matching Scopes - Denied",
			claims: &CustomClaims{
				Scopes: []string{"read:metrics", "write:logs"},
			},
			action:   "publish",
			channel:  "user.updates",
			expected: false,
			authIsEnabled: true,
		},
		{
			name: "Multiple Scopes - One Match - Allowed",
			claims: &CustomClaims{
				Scopes: []string{"read:metrics", "publish:user.*", "write:logs"},
			},
			action:   "publish",
			channel:  "user.feed.abc",
			expected: true,
			authIsEnabled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session := &ClientSession{}
			if tc.authIsEnabled {
				session.claims = tc.claims
			}

			assert.Equal(t, tc.expected, session.CanAccess(tc.action, tc.channel))
		})
	}
}
