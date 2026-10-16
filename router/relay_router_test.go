package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

// wind-tide: every route that starts a generation counts against the model
// rate limit, the playground included; reading a task does not
// (wind-tide/API.md §10).
func TestWtGenerationRoutesCountAgainstTheModelRateLimit(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}))
	require.NoError(t, i18n.Init())
	previousEnabled, previousMinutes, previousCount := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount = previousEnabled, previousMinutes, previousCount
	})
	setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes, setting.ModelRequestRateLimitCount = true, 1, 1

	engine := gin.New()
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	SetTaskRouter(engine)
	serve := func(method, path, credential string) int {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(`{"model":"wt-test","prompt":"tide"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+credential)
		engine.ServeHTTP(recorder, request)
		return recorder.Code
	}

	// The in-memory limiter lives as long as the test binary, so every account
	// gets an ID no earlier run used.
	firstID := 3_000_000 + int(time.Now().UnixNano()%1_000_000_000)
	var overLimitKey string
	for i, route := range []struct {
		path    string
		session bool
	}{
		{"/pg/chat/completions", true},
		{"/mj/submit/imagine", false},
		{"/fast/mj/submit/imagine", false},
		{"/mj/insight-face/swap", false},
		{"/v1/tasks/wt-test", false},
		{"/v1/video/generations", false},
		{"/v1/videos", false},
		{"/v1/videos/video-1/remix", false},
	} {
		user := model.User{Id: firstID + i, Username: fmt.Sprintf("wt-limited-%d", i), AffCode: fmt.Sprintf("wt-limited-%d", i), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
		require.NoError(t, model.DB.Create(&user).Error)
		key := fmt.Sprintf("wtlimited%d", user.Id)
		require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: key, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
		credential := "sk-" + key
		if route.session {
			bundle, err := service.CreateLoginSession(user.Id, "oauth:discord", "127.0.0.1", "wind-tide-test")
			require.NoError(t, err)
			credential = bundle.AccessToken
		}
		assert.NotEqual(t, http.StatusTooManyRequests, serve(http.MethodPost, route.path, credential), route.path)
		assert.Equal(t, http.StatusTooManyRequests, serve(http.MethodPost, route.path, credential), route.path)
		overLimitKey = credential
	}
	for _, read := range []string{"/mj/task/1/fetch", "/v1/video/generations/task-1"} {
		assert.NotEqual(t, http.StatusTooManyRequests, serve(http.MethodGet, read, overLimitKey), read)
	}
}

func setupRelayRouterTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}))

	t.Cleanup(func() {
		if sqlDB, err := model.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
}
