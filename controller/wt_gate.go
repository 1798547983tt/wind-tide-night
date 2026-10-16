package controller

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// wind-tide: the Discord sign-in gate and the emergency login
// (wind-tide/API.md §3 and §4).

// wtOAuthLoginCookie holds the nonce that binds a Discord sign-in to the
// browser that started it, so nobody can sign a victim in to the attacker's
// account by sending a callback link (login CSRF).
const wtOAuthLoginCookie = "wt_oauth_login"

// newWtOAuthLoginCookie is the sign-in cookie with a value and a lifetime.
// Over HTTPS its name carries the __Host- prefix, which browsers accept only
// on a Secure cookie with Path=/ and no Domain, so neither a sibling
// subdomain nor a plain-HTTP response can plant or overwrite it.
func newWtOAuthLoginCookie(value string, maxAge int) *http.Cookie {
	cookie := &http.Cookie{
		Name:     wtOAuthLoginCookie,
		Value:    value,
		Path:     "/api/oauth",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if common.SessionCookieSecure {
		cookie.Name, cookie.Path, cookie.Secure = "__Host-"+wtOAuthLoginCookie, "/", true
	}
	return cookie
}

// wtGateMessages answer a Discord account the gate does not admit.
var wtGateMessages = map[string]string{
	model.WtEntryPending:  "Your entry request is waiting for approval",
	model.WtEntryRejected: "Your entry request was rejected, please try again later",
	model.WtGateRemoved:   "Your access to this site has been removed",
}

// wtHandleDiscordLogin replaces the upstream find-or-create step for Discord:
// an admitted account signs in through the shared login policy, any other
// account gets its gate state with HTTP 200 and no session.
func wtHandleDiscordLogin(c *gin.Context, oauthUser *oauth.OAuthUser) {
	avatarHash, _ := oauthUser.Extra["avatar"].(string)
	identity := model.WtDiscordIdentity{
		DiscordId:  oauthUser.ProviderUserID,
		Username:   oauthUser.Username,
		GlobalName: oauthUser.DisplayName,
		AvatarHash: avatarHash,
	}
	decision, err := model.WtAdmitDiscordAccount(identity, time.Now())
	if err != nil {
		var message *common.Message
		if errors.As(err, &message) {
			common.ApiError(c, err)
			return
		}
		writeSecurityOperationError(c, err)
		return
	}
	service.WtRefreshAvatarAsync(identity.DiscordId, avatarHash)
	if decision.PinnedEntryId != 0 && decision.User != nil {
		params := map[string]any{"id": decision.PinnedEntryId, "discord_id": identity.DiscordId, "discord_username": identity.Username}
		model.RecordOperationAuditLog(decision.User.Id, decision.User.Role, auditContentEN("wt.allowlist_pin", params), c.ClientIP(), "wt.allowlist_pin", params, nil, nil, c)
	}
	if decision.User == nil {
		setAuthNoStore(c)
		common.ApiErrorStatus(c, http.StatusOK, common.NewMessage(wtGateMessages[decision.Gate]), gin.H{"data": gin.H{
			"wt_gate": decision.Gate,
			"applicant": gin.H{
				"discord_id":   identity.DiscordId,
				"username":     identity.Username,
				"display_name": identity.DisplayName(),
				"avatar_url":   model.WtAvatarURL(identity.DiscordId, avatarHash),
			},
			"requested_at": decision.RequestedAt,
			"retry_after":  decision.RetryAfter,
		}})
		return
	}
	if decision.User.Status != common.UserStatusEnabled {
		common.ApiErrorT(c, "User has been banned")
		return
	}
	setupLogin(decision.User, nil, c)
}

// WtEmergencyLogin signs the owner in with a single-use emergency token from
// `wt emergency-login`. Every failure gets the same answer; neither the token
// nor its digest is logged.
func WtEmergencyLogin(c *gin.Context) {
	var request struct {
		Token string `json:"token"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		request.Token = ""
	}
	setAuthNoStore(c)
	owner, err := model.WtOwnerAccount()
	if err == nil {
		err = model.ConsumeWtEmergencyToken(request.Token, time.Now())
	}
	if err != nil {
		if !errors.Is(err, model.ErrWtEmergencyTokenInvalid) {
			common.SysError(common.LogText("emergency login error: %s", err.Error()))
		}
		common.SysLog(common.LogText("rejected an emergency login attempt from %s", c.ClientIP()))
		// The attempt is anonymous, so the audit entry has no actor role and
		// only root sees it.
		ownerID := 0
		if owner != nil {
			ownerID = owner.Id
		}
		model.RecordOperationAuditLog(ownerID, 0, auditContentEN("wt.emergency_login_failed", nil), c.ClientIP(), "wt.emergency_login_failed", map[string]any{}, nil,
			&model.AuditRequestInfo{Method: c.Request.Method, Route: c.FullPath(), Path: c.FullPath(), Status: http.StatusOK, Success: false}, c)
		common.ApiError(c, model.ErrWtEmergencyTokenInvalid)
		return
	}
	model.RecordOperationAuditLog(owner.Id, owner.Role, auditContentEN("wt.emergency_login", nil), c.ClientIP(), "wt.emergency_login", map[string]any{}, nil, nil, c)
	c.Set("login_method", "emergency")
	setupLogin(owner, nil, c)
}

// wtBindOAuthLogin gives the browser that starts a Discord sign-in a random
// nonce in an HttpOnly cookie and returns the nonce's digest for the sign-in
// flow. Starting another sign-in replaces the nonce.
func wtBindOAuthLogin(c *gin.Context) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(c.Writer, newWtOAuthLoginCookie(nonce, int(oauthAuthFlowTTL/time.Second)))
	digest := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(digest[:]), nil
}

// wtCheckOAuthLogin clears the sign-in cookie and reports whether it holds
// the nonce of the flow's binding.
func wtCheckOAuthLogin(c *gin.Context, binding string) bool {
	cleared := newWtOAuthLoginCookie("", -1)
	cleared.Expires = time.Unix(1, 0)
	nonce, err := c.Cookie(cleared.Name)
	http.SetCookie(c.Writer, cleared)
	if err != nil || nonce == "" || binding == "" {
		return false
	}
	digest := sha256.Sum256([]byte(nonce))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(binding)) == 1
}
