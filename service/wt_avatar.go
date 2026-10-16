package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

// wind-tide: local cache of Discord avatars (wind-tide/API.md §8). Avatars are
// fetched only from the Discord CDN, only for validated IDs and hashes, and the
// site serves the cached files itself.

const (
	wtAvatarMaxBytes     = 1 << 20
	wtAvatarRetryBackoff = 10 * time.Minute
)

var (
	wtAvatarClient = &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	wtPNGSignature = []byte("\x89PNG\r\n\x1a\n")

	// wtAvatarInflight and wtAvatarFailures are keyed by "{id}_{hash}".
	wtAvatarInflight sync.Map
	wtAvatarFailures sync.Map
)

// WtAvatarDir is the avatar cache directory, WT_AVATAR_DIR or ./wt-avatars.
func WtAvatarDir() string {
	if dir := strings.TrimSpace(os.Getenv("WT_AVATAR_DIR")); dir != "" {
		return dir
	}
	return "./wt-avatars"
}

// WtAvatarPath is the cache file of one avatar version.
func WtAvatarPath(discordID, hash string) string {
	return filepath.Join(WtAvatarDir(), discordID+"_"+hash+".png")
}

// WtRefreshAvatarAsync caches an avatar in the background unless it is cached
// already, being downloaded, or failed less than wtAvatarRetryBackoff ago.
func WtRefreshAvatarAsync(discordID, hash string) {
	if !model.WtValidDiscordID(discordID) || !model.WtValidAvatarHash(hash) {
		return
	}
	path := WtAvatarPath(discordID, hash)
	if _, err := os.Stat(path); err == nil {
		return
	}
	key := discordID + "_" + hash
	if failedAt, ok := wtAvatarFailures.Load(key); ok && time.Since(failedAt.(time.Time)) < wtAvatarRetryBackoff {
		return
	}
	if _, running := wtAvatarInflight.LoadOrStore(key, struct{}{}); running {
		return
	}
	gopool.Go(func() {
		defer wtAvatarInflight.Delete(key)
		if err := downloadWtAvatar(discordID, hash, path); err != nil {
			wtAvatarFailures.Store(key, time.Now())
			common.SysError(common.LogText("failed to cache the Discord avatar of %s: %s", discordID, err.Error()))
			return
		}
		wtAvatarFailures.Delete(key)
	})
}

// downloadWtAvatar stores the 128px avatar from the Discord CDN when it is a
// PNG within wtAvatarMaxBytes, then drops the older cached versions.
func downloadWtAvatar(discordID, hash, path string) error {
	source := "https://cdn.discordapp.com/avatars/" + discordID + "/" + hash + ".png?size=128"
	resp, err := wtAvatarClient.Get(source)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, wtAvatarMaxBytes+1))
	if err != nil {
		return err
	}
	if len(body) > wtAvatarMaxBytes || !bytes.HasPrefix(body, wtPNGSignature) {
		return errors.New("the response is not a PNG image within the size limit")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".avatar-*.tmp")
	if err != nil {
		return err
	}
	_, writeErr := temp.Write(body)
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	older, err := filepath.Glob(filepath.Join(dir, discordID+"_*.png"))
	if err != nil {
		return nil
	}
	for _, file := range older {
		if file != path {
			_ = os.Remove(file)
		}
	}
	return nil
}
