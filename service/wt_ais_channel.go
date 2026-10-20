package service

import (
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// wind-tide: the site owns the relay side of the donated-account pools
// (wind-tide/API.md §15): the "AIS PG" and "BUILD" groups, the ratios of
// their relay models and the channels tagged wt-ais and wt-build that forward
// them to aistudio-api and AIStudioToAPI. Every start puts them back; the
// owner may change anything else in the admin UI.

// wtPoolChannel is the relay side of one pool as the site wants it.
type wtPoolChannel struct {
	provider   string
	group      string
	groupLabel string
	name       string
	tag        string
	baseURL    string
	key        string
	log        string
}

func wtAisPoolChannel() wtPoolChannel {
	return wtPoolChannel{provider: model.WtAisProvider, group: WtAisGroup, groupLabel: WtAisGroupLabel, name: WtAisChannelName, tag: WtAisChannelTag, baseURL: wtAisURL(), key: WtAisAPIKey(), log: "wt ais"}
}

func wtBuildPoolChannel() wtPoolChannel {
	return wtPoolChannel{provider: model.WtBuildProvider, group: WtBuildGroup, groupLabel: WtBuildGroupLabel, name: WtBuildChannelName, tag: WtBuildChannelTag, baseURL: wtBuildURL(), key: WtBuildAPIKey(), log: "wt build"}
}

// WtEnsureAisChannel creates or repairs the group, the ratios and the
// channel of the AIS playground pool. It does nothing while the pool is not
// configured and is idempotent; one log line says what it did.
func WtEnsureAisChannel() error {
	if !WtAisEnabled() {
		return nil
	}
	return wtEnsurePoolChannel(wtAisPoolChannel())
}

// WtEnsureBuildChannel is WtEnsureAisChannel for the BUILD pool.
func WtEnsureBuildChannel() error {
	if !WtBuildEnabled() {
		return nil
	}
	return wtEnsurePoolChannel(wtBuildPoolChannel())
}

func wtEnsurePoolChannel(pool wtPoolChannel) error {
	specs := model.WtAisModelsOf(pool.provider)
	var changes []string
	groupRatios := ratio_setting.GetGroupRatioCopy()
	if _, ok := groupRatios[pool.group]; !ok {
		groupRatios[pool.group] = 1
		if err := wtUpdateOptionJSON("GroupRatio", groupRatios); err != nil {
			return err
		}
		changes = append(changes, "group ratio")
	}
	usableGroups := setting.GetUserUsableGroupsCopy()
	if _, ok := usableGroups[pool.group]; !ok {
		usableGroups[pool.group] = pool.groupLabel
		if err := wtUpdateOptionJSON("UserUsableGroups", usableGroups); err != nil {
			return err
		}
		changes = append(changes, "usable groups")
	}
	// Each relay model bills like its upstream model; a model without a ratio
	// of its own bills at 1.
	for _, ratios := range []struct {
		option string
		values map[string]float64
	}{
		{"ModelRatio", ratio_setting.GetModelRatioCopy()},
		{"CompletionRatio", ratio_setting.GetCompletionRatioCopy()},
	} {
		changed := false
		for _, spec := range specs {
			want := 1.0
			if upstream, ok := ratios.values[spec.Upstream]; ok {
				want = upstream
			}
			if current, ok := ratios.values[spec.Name]; ok && current == want {
				continue
			}
			ratios.values[spec.Name] = want
			changed = true
		}
		if !changed {
			continue
		}
		if err := wtUpdateOptionJSON(ratios.option, ratios.values); err != nil {
			return err
		}
		changes = append(changes, strings.ToLower(ratios.option))
	}

	mapping, err := common.Marshal(wtPoolModelMapping(specs))
	if err != nil {
		return err
	}
	channels, err := model.GetChannelsByTag(pool.tag, true, true)
	if err != nil {
		return err
	}
	var channel *model.Channel
	for _, candidate := range channels {
		if channel == nil || candidate.Id < channel.Id {
			channel = candidate
		}
	}
	switch {
	case channel == nil:
		channel = &model.Channel{CreatedTime: common.GetTimestamp()}
		channel.SetTag(pool.tag)
		wtApplyPoolChannel(channel, pool, specs, string(mapping))
		if err := channel.Insert(); err != nil {
			return err
		}
		changes = append(changes, "channel created")
	case !wtPoolChannelMatches(channel, pool, specs):
		wtApplyPoolChannel(channel, pool, specs, string(mapping))
		if err := channel.Update(); err != nil {
			return err
		}
		changes = append(changes, "channel updated")
	}
	if len(changes) > 0 {
		model.InitChannelCache()
	}
	summary := "unchanged"
	if len(changes) > 0 {
		summary = strings.Join(changes, ", ")
	}
	common.SysLog(common.LogText("%s: channel %s: %s", pool.log, pool.name, summary))
	return nil
}

// wtUpdateOptionJSON stores a map option the way the admin UI saves it.
func wtUpdateOptionJSON(key string, value any) error {
	encoded, err := common.Marshal(value)
	if err != nil {
		return err
	}
	return model.UpdateOption(key, string(encoded))
}

// wtPoolModelNames is the channel's model list, relay names in table order.
func wtPoolModelNames(specs []model.WtAisModelSpec) string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return strings.Join(names, ",")
}

// wtPoolModelMapping maps each relay model to the model the pool serves.
func wtPoolModelMapping(specs []model.WtAisModelSpec) map[string]string {
	mapping := make(map[string]string, len(specs))
	for _, spec := range specs {
		mapping[spec.Name] = spec.Upstream
	}
	return mapping
}

// wtApplyPoolChannel writes the fields the site owns on the pool's channel.
func wtApplyPoolChannel(channel *model.Channel, pool wtPoolChannel, specs []model.WtAisModelSpec, mapping string) {
	baseURL, priority, weight := pool.baseURL, int64(0), uint(0)
	channel.Type = constant.ChannelTypeGemini
	channel.Key = pool.key
	channel.BaseURL = &baseURL
	channel.Models = wtPoolModelNames(specs)
	channel.Group = "default,vip,svip," + pool.group
	channel.ModelMapping = &mapping
	channel.Name = pool.name
	channel.Status = common.ChannelStatusEnabled
	channel.Priority = &priority
	channel.Weight = &weight
}

// wtPoolChannelMatches reports whether the fields the site owns are as the
// site wants them. The mapping is compared as a map, so the order its JSON
// was written in does not matter.
func wtPoolChannelMatches(channel *model.Channel, pool wtPoolChannel, specs []model.WtAisModelSpec) bool {
	var mapping map[string]string
	if common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping) != nil {
		return false
	}
	return channel.Type == constant.ChannelTypeGemini &&
		channel.Key == pool.key &&
		channel.BaseURL != nil && *channel.BaseURL == pool.baseURL &&
		channel.Models == wtPoolModelNames(specs) &&
		channel.Group == "default,vip,svip,"+pool.group &&
		maps.Equal(mapping, wtPoolModelMapping(specs)) &&
		channel.Name == pool.name &&
		channel.Status == common.ChannelStatusEnabled &&
		channel.GetPriority() == 0 &&
		channel.GetWeight() == 0
}
