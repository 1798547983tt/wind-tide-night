package service

import (
	"context"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// wind-tide: the relay side of the WorkBuddy pool (wind-tide/API.md §16):
// one channel tagged wt-wb that forwards to the workbuddy2api panel. Unlike
// the AIS PG and BUILD channels, this one is the owner's after its birth:
// the site creates it once (type OpenAI, the panel's key and address, every
// member group, the models the panel serves under WB- names mapped back to
// the panel's ids, a ratio of 1 for each) and on later starts repairs only
// the type, the key, the address and the name. While the channel still
// carries the seed list it was born with, a panel that has learned its
// models replaces it; a list the owner edited is never touched.

const (
	// WtWbModelPrefix is what the panel's models are called on the site.
	WtWbModelPrefix = "WB-"
	// wtWbMemberGroups are the groups every member is in; the pool has no
	// gate, so there is no group of its own.
	wtWbMemberGroups = "default,vip,svip"
)

// wtWbSeedModels is the list the channel starts with while the panel serves
// nothing yet (it lists models only once it holds an account): the models
// of workbuddy2api's own catalog, as /v1/models names them, CodeBuddy's
// under cn: and WorkBuddy's under global:.
var wtWbSeedModels = []string{
	"cn:glm-5.3", "cn:glm-5.3-flash", "cn:glm-5.2", "cn:glm-5.1", "cn:glm-5v-turbo",
	"cn:kimi-k3", "cn:kimi-k2.7", "cn:kimi-k2.6",
	"cn:minimax-m3", "cn:hy4-preview", "cn:hy4-preview-x", "cn:hy3", "cn:hy3-preview",
	"cn:deepseek-v4-pro", "cn:deepseek-v4-flash", "cn:deepseek-v4.1-flash",
	"global:gpt-6-astra", "global:gpt-5.6-sol", "global:gpt-5.6-terra", "global:gpt-5.6-luna",
	"global:gpt-5.5", "global:gpt-5.4", "global:gpt-5.3-codex",
	"global:gemini-3.5-flash", "global:kimi-k2.8-preview", "global:kimi-k2.5",
}

// WtWbRelayName is the site's name of a panel model: WB- and the id, with
// the cn: realm dropped (it is the default) and global: kept as a word.
func WtWbRelayName(id string) string {
	name := strings.TrimPrefix(id, WtWbRealmCN+":")
	name = strings.ReplaceAll(name, ":", "-")
	return WtWbModelPrefix + name
}

// wtWbUpstreamName is the model a panel id stands for, without its realm,
// for the ratio it bills like.
func wtWbUpstreamName(id string) string {
	if _, bare, ok := strings.Cut(id, ":"); ok {
		return bare
	}
	return id
}

// wtWbModelIDs is the panel's list cleaned: ids that would make a bad relay
// name are dropped, duplicates kept once, order kept.
func wtWbModelIDs(ids []string) []string {
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || strings.ContainsAny(id, ", \n\t\"") || slices.Contains(clean, id) {
			continue
		}
		clean = append(clean, id)
	}
	return clean
}

// wtWbModelList is the channel's Models field for a list of panel ids.
func wtWbModelList(ids []string) string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, WtWbRelayName(id))
	}
	return strings.Join(names, ",")
}

// wtWbModelMapping maps each relay name to the panel id it stands for.
func wtWbModelMapping(ids []string) map[string]string {
	mapping := make(map[string]string, len(ids))
	for _, id := range ids {
		mapping[WtWbRelayName(id)] = id
	}
	return mapping
}

// WtEnsureWbChannel creates the WorkBuddy channel once and repairs its
// connection fields on later starts. It does nothing while the pool is not
// configured and is idempotent; one log line says what it did.
func WtEnsureWbChannel() error {
	if !WtWbEnabled() {
		return nil
	}
	channels, err := model.GetChannelsByTag(WtWbChannelTag, true, true)
	if err != nil {
		return err
	}
	var channel *model.Channel
	for _, candidate := range channels {
		if channel == nil || candidate.Id < channel.Id {
			channel = candidate
		}
	}
	var changes []string
	ids := wtWbServedModels()
	switch {
	case channel == nil:
		if len(ids) == 0 {
			ids = wtWbSeedModels
		}
		mapping, err := common.Marshal(wtWbModelMapping(ids))
		if err != nil {
			return err
		}
		baseURL, priority, weight, encoded := wtWbURL(), int64(0), uint(0), string(mapping)
		channel = &model.Channel{
			Type:         constant.ChannelTypeOpenAI,
			Key:          WtWbAPIKey(),
			BaseURL:      &baseURL,
			Models:       wtWbModelList(ids),
			Group:        wtWbMemberGroups,
			ModelMapping: &encoded,
			Name:         WtWbChannelName,
			Status:       common.ChannelStatusEnabled,
			Priority:     &priority,
			Weight:       &weight,
			CreatedTime:  common.GetTimestamp(),
		}
		channel.SetTag(WtWbChannelTag)
		if err := channel.Insert(); err != nil {
			return err
		}
		changes = append(changes, "channel created")
	default:
		if channel.Type != constant.ChannelTypeOpenAI || channel.Key != WtWbAPIKey() || channel.GetBaseURL() != wtWbURL() || channel.Name != WtWbChannelName {
			baseURL := wtWbURL()
			channel.Type = constant.ChannelTypeOpenAI
			channel.Key = WtWbAPIKey()
			channel.BaseURL = &baseURL
			channel.Name = WtWbChannelName
			changes = append(changes, "connection repaired")
		}
		if len(ids) > 0 && channel.Models == wtWbModelList(wtWbSeedModels) && channel.Models != wtWbModelList(ids) {
			mapping, err := common.Marshal(wtWbModelMapping(ids))
			if err != nil {
				return err
			}
			encoded := string(mapping)
			channel.Models = wtWbModelList(ids)
			channel.ModelMapping = &encoded
			changes = append(changes, "seed list replaced by the panel's models")
		}
		if len(changes) > 0 {
			if err := channel.Update(); err != nil {
				return err
			}
		}
	}
	seeded, err := wtWbSeedRatios(channel)
	if err != nil {
		return err
	}
	if seeded {
		changes = append(changes, "ratios")
	}
	if len(changes) > 0 {
		model.InitChannelCache()
	}
	summary := "unchanged"
	if len(changes) > 0 {
		summary = strings.Join(changes, ", ")
	}
	common.SysLog(common.LogText("wt wb: channel %s: %s", WtWbChannelName, summary))
	return nil
}

// wtWbServedModels is what the panel lists right now; nothing when it does
// not answer, which the start log already says.
func wtWbServedModels() []string {
	ctx, cancel := context.WithTimeout(context.Background(), wtWbTimeout)
	defer cancel()
	ids, err := WtWbListModels(ctx)
	if err != nil {
		return nil
	}
	return wtWbModelIDs(ids)
}

// wtWbSeedRatios gives every WB- model of the channel that has no ratio yet
// the ratio of the model it stands for, or 1. Ratios the owner set stay.
func wtWbSeedRatios(channel *model.Channel) (bool, error) {
	var mapping map[string]string
	_ = common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping)
	seeded := false
	for _, ratios := range []struct {
		option string
		values map[string]float64
	}{
		{"ModelRatio", ratio_setting.GetModelRatioCopy()},
		{"CompletionRatio", ratio_setting.GetCompletionRatioCopy()},
	} {
		changed := false
		for _, name := range strings.Split(channel.Models, ",") {
			name = strings.TrimSpace(name)
			if !strings.HasPrefix(name, WtWbModelPrefix) {
				continue
			}
			if _, ok := ratios.values[name]; ok {
				continue
			}
			want := 1.0
			if upstream, ok := ratios.values[wtWbUpstreamName(mapping[name])]; ok && mapping[name] != "" {
				want = upstream
			}
			ratios.values[name] = want
			changed = true
		}
		if !changed {
			continue
		}
		if err := wtUpdateOptionJSON(ratios.option, ratios.values); err != nil {
			return seeded, err
		}
		seeded = true
	}
	return seeded, nil
}
