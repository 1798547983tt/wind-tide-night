package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// wind-tide: Discord is the only way into the site (wind-tide/API.md §3). The
// refusals below hold whatever the options say. Discord OAuth stays available
// for sign-in and for security verification, never for linking an account.
// No account has a password: one would only take Discord's place as the
// account's step-up verification.

// WtDiscordOnlyMessage answers every refused entry point.
const WtDiscordOnlyMessage = "This site only supports signing in with Discord"

// wtBodyLimit bounds a request body the policy reads before the route's own
// body limit; the requests it reads are a few hundred bytes.
const wtBodyLimit = 64 << 10

// wtRefusedRoutes are the password, email-code, passkey, WeChat and Telegram
// sign-in, registration and binding routes, the setup wizard and the
// administrators' account creation, keyed by method and gin full path.
var wtRefusedRoutes = map[string]bool{
	"POST /api/setup":                          true,
	"GET /api/verification":                    true,
	"GET /api/reset_password":                  true,
	"POST /api/user/reset":                     true,
	"POST /api/user/register":                  true,
	"POST /api/user/login":                     true,
	"POST /api/user/passkey/login/begin":       true,
	"POST /api/user/passkey/login/finish":      true,
	"POST /api/user/":                          true,
	"GET /api/oauth/wechat":                    true,
	"POST /api/oauth/wechat/bind":              true,
	"GET /api/oauth/telegram/login":            true,
	"POST /api/oauth/telegram/bind/start":      true,
	"GET /api/oauth/telegram/bind/:flow_token": true,
}

// WtDiscordOnly refuses the sign-in, registration and account-linking entry
// points other than Discord sign-in, OAuth flows of any other provider or for
// linking, clearing an account's Discord binding, and setting a password.
func WtDiscordOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		route := c.Request.Method + " " + c.FullPath()
		refused := wtRefusedRoutes[route]
		switch route {
		case "POST /api/oauth/state":
			body, ok := wtReadBody(c)
			if !ok {
				return
			}
			// Decoded exactly like GenerateOAuthCode; anything it cannot decode
			// is refused here as well.
			var request struct {
				Provider string `json:"provider"`
				Intent   string `json:"intent"`
			}
			refused = common.DecodeJson(bytes.NewReader(body), &request) != nil ||
				strings.TrimSpace(request.Provider) != "discord" ||
				strings.TrimSpace(request.Intent) == model.AuthFlowIntentBind
		case "GET /api/oauth/:provider":
			refused = c.Param("provider") != "discord"
			if !refused {
				// A linking flow started before this policy cannot finish.
				flow, err := model.GetAuthFlow(c.Query("state"), model.AuthFlowMatch{Purpose: model.AuthFlowPurposeOAuth, Provider: "discord"})
				refused = err == nil && flow.Intent == model.AuthFlowIntentBind
			}
		case "DELETE /api/user/:id/bindings/:binding_type":
			// Normalized like AdminClearUserBinding: Unicode lower-casing maps
			// more spellings to "discord" than case folding does.
			refused = strings.ToLower(strings.TrimSpace(c.Param("binding_type"))) == "discord"
		case "PUT /api/user/self", "PUT /api/user/":
			body, ok := wtReadBody(c)
			if !ok {
				return
			}
			// UpdateSelf and UpdateUser decode the body with encoding/json,
			// which matches a key to the password field ignoring case the way
			// strings.EqualFold does, so every such key counts. Anything they
			// cannot decode is refused here as well.
			var fields map[string]any
			refused = common.DecodeJson(bytes.NewReader(body), &fields) != nil
			for key, value := range fields {
				if !strings.EqualFold(key, "password") || value == nil {
					continue
				}
				if password, isString := value.(string); !isString || password != "" {
					refused = true
				}
			}
		}
		if refused {
			abortWithWebMessage(c, http.StatusForbidden, "WT_DISCORD_ONLY", common.NewMessage(WtDiscordOnlyMessage))
			return
		}
		c.Next()
	}
}

// wtReadBody reads a request body the policy decides on, up to wtBodyLimit,
// and puts it back for the route's handler. When the body cannot be read or
// is too large it answers the request and returns false.
func wtReadBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, wtBodyLimit+1))
	_ = c.Request.Body.Close()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return nil, false
	}
	if len(body) > wtBodyLimit {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
		return nil, false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	return body, true
}
